package root

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

type SubjectIdentity struct {
	FormatVersion  int                   `json:"format_version"`
	SubjectID      string                `json:"subject_id"`
	DirectoryName  string                `json:"directory_name"`
	RuntimeProfile domain.RuntimeProfile `json:"runtime_profile"`
	SessionID      string                `json:"session_id"`
	CreatedAt      time.Time             `json:"created_at"`
}

func DirectoryName(now time.Time, id string) string {
	return now.UTC().Format("20060102T150405Z") + "_" + id
}

func (o *Owner) MaterializeSubject(ctx context.Context, subject domain.Subject) (string, error) {
	finalPath, err := o.SubjectPath(subject.DirectoryName)
	if err != nil { return "", err }
	identity := SubjectIdentity{FormatVersion:1,SubjectID:subject.ID,DirectoryName:subject.DirectoryName,RuntimeProfile:subject.Profile,SessionID:subject.SessionID,CreatedAt:subject.CreatedAt.UTC()}
	if existing, readErr := ReadSubjectIdentity(finalPath); readErr == nil {
		if existing.SubjectID != subject.ID || existing.DirectoryName != subject.DirectoryName || !existing.RuntimeProfile.Equal(subject.Profile) {
			return "", domain.NewError("WORKSPACE_UNAVAILABLE","subject identity conflicts with database",http.StatusConflict)
		}
		if err=o.Store.MarkMaterialized(ctx,subject.ID,time.Now().UTC());err!=nil{return "",err}
		return finalPath,nil
	} else if !errors.Is(readErr,os.ErrNotExist) {
		return "",readErr
	}
	stage:=filepath.Join(o.Control,"staging","subject-"+subject.ID+"-"+uuid.NewString())
	defer os.RemoveAll(stage)
	for _,rel:=range []string{".nornsd/config/agent",".nornsd/runtimes",".nornsd/log",".nornsd/staging"}{if err=os.MkdirAll(filepath.Join(stage,filepath.FromSlash(rel)),0700);err!=nil{return "",err}}
	if err=writeJSONAtomic(filepath.Join(stage,".nornsd","subject.json"),identity,0600);err!=nil{return "",err}
	for _,name:=range []string{"raw.jsonl","readable.jsonl"}{file,createErr:=os.OpenFile(filepath.Join(stage,".nornsd","log",name),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if createErr!=nil{return "",createErr};if closeErr:=file.Close();closeErr!=nil{return "",closeErr}}
	if err=os.Rename(stage,finalPath);err!=nil{return "",domain.Wrap("WORKSPACE_UNAVAILABLE","cannot publish subject workspace",http.StatusConflict,err)}
	if err=o.Store.MarkMaterialized(ctx,subject.ID,time.Now().UTC());err!=nil{return "",err}
	return finalPath,nil
}

func ReadSubjectIdentity(subjectPath string)(SubjectIdentity,error){var id SubjectIdentity;b,err:=os.ReadFile(filepath.Join(subjectPath,".nornsd","subject.json"));if err!=nil{return id,err};if err=json.Unmarshal(b,&id);err!=nil{return id,err};if id.FormatVersion!=1||id.SubjectID==""||id.DirectoryName==""||id.SessionID==""{return id,fmt.Errorf("invalid subject identity")};return id,nil}

func EnsureWithin(base,target string)(string,error){baseAbs,err:=filepath.Abs(base);if err!=nil{return "",err};targetAbs,err:=filepath.Abs(target);if err!=nil{return "",err};rel,err:=filepath.Rel(baseAbs,targetAbs);if err!=nil||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator))||filepath.IsAbs(rel){return "",fmt.Errorf("path escapes allowed root")};return targetAbs,nil}

func CopyTreeNoLinks(source,target string)error{
	info,err:=os.Stat(source);if err!=nil{return err};if !info.IsDir(){return fmt.Errorf("source is not a directory")}
	return filepath.WalkDir(source,func(path string,d fs.DirEntry,walkErr error)error{if walkErr!=nil{return walkErr};rel,err:=filepath.Rel(source,path);if err!=nil{return err};if d.Type()&os.ModeSymlink!=0{return fmt.Errorf("symbolic link is not allowed: %s",path)};if rel=="."{return os.MkdirAll(target,0700)};dst:=filepath.Join(target,rel);if d.IsDir(){return os.MkdirAll(dst,0700)};b,err:=os.ReadFile(path);if err!=nil{return err};return os.WriteFile(dst,b,0600)})
}
