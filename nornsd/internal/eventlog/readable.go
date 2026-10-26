package eventlog

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

var ErrInvalidReadableCursor = errors.New("invalid readable event cursor")

type ReadableCursor struct {
	Offset int64
	Seq    uint64
	Hash   string
}

type ReadablePage struct {
	Events  []domain.Event
	Cursor  ReadableCursor
	Older   *ReadableCursor
	HasMore bool
}

type readableRecord struct {
	event domain.Event
	start int64
	end   int64
	hash  string
}

func ReadableLatest(path string, limit int) (ReadablePage, error) {
	file, err := os.Open(path)
	if err != nil {
		return ReadablePage{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return ReadablePage{}, err
	}
	end, err := lastCompleteOffset(file, info.Size())
	if err != nil {
		return ReadablePage{}, err
	}
	records, err := readRecordsBefore(file, end, limit+1)
	if err != nil {
		return ReadablePage{}, err
	}
	page := ReadablePage{Events: []domain.Event{}}
	if len(records) > 0 {
		last := records[len(records)-1]
		page.Cursor = ReadableCursor{Offset: last.end, Seq: last.event.Seq, Hash: last.hash}
	}
	if len(records) > limit {
		records = records[1:]
		page.Older = &ReadableCursor{Offset: records[0].start, Seq: records[0].event.Seq, Hash: records[0].hash}
		page.HasMore = true
	}
	for _, record := range records {
		page.Events = append(page.Events, record.event)
	}
	return page, nil
}

func ReadableBefore(path string, cursor ReadableCursor, limit int) (ReadablePage, error) {
	file, err := os.Open(path)
	if err != nil {
		return ReadablePage{}, err
	}
	defer file.Close()

	boundary, complete, err := readRecord(bufio.NewReaderSize(io.NewSectionReader(file, cursor.Offset, version.MaxAgentFrameBytes+1024*1024+1), 64*1024), cursor.Offset)
	if err != nil && !errors.Is(err, io.EOF) || !complete || boundary.event.Seq != cursor.Seq || boundary.hash != cursor.Hash {
		return ReadablePage{}, ErrInvalidReadableCursor
	}
	records, err := readRecordsBefore(file, cursor.Offset, limit+1)
	if err != nil {
		return ReadablePage{}, err
	}
	if len(records) > 0 && records[len(records)-1].event.Seq+1 != boundary.event.Seq {
		return ReadablePage{}, fmt.Errorf("readable event sequence gap before %d", boundary.event.Seq)
	}
	page := ReadablePage{Events: []domain.Event{}}
	if len(records) > limit {
		records = records[1:]
		page.Older = &ReadableCursor{Offset: records[0].start, Seq: records[0].event.Seq, Hash: records[0].hash}
		page.HasMore = true
	}
	for _, record := range records {
		page.Events = append(page.Events, record.event)
	}
	return page, nil
}

func ReadableAfter(path string, cursor ReadableCursor, limit int) (ReadablePage, error) {
	file, err := os.Open(path)
	if err != nil {
		return ReadablePage{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ReadablePage{}, err
	}
	if cursor.Offset < 0 || cursor.Offset > info.Size() {
		return ReadablePage{}, ErrInvalidReadableCursor
	}
	if err = validateReadableCursor(file, cursor); err != nil {
		return ReadablePage{}, err
	}
	if _, err = file.Seek(cursor.Offset, io.SeekStart); err != nil {
		return ReadablePage{}, err
	}

	page := ReadablePage{Events: []domain.Event{}, Cursor: cursor}
	records := make([]readableRecord, 0, limit+1)
	reader := bufio.NewReaderSize(file, 64*1024)
	offset := cursor.Offset
	previous := cursor.Seq
	for len(records) <= limit {
		record, complete, readErr := readRecord(reader, offset)
		if complete {
			if record.event.Seq != previous+1 {
				return ReadablePage{}, ErrInvalidReadableCursor
			}
			previous = record.event.Seq
			offset = record.end
			records = append(records, record)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return ReadablePage{}, readErr
		}
	}
	if len(records) > limit {
		records = records[:limit]
		page.HasMore = true
	}
	for _, record := range records {
		page.Events = append(page.Events, record.event)
	}
	if len(records) > 0 {
		last := records[len(records)-1]
		page.Cursor = ReadableCursor{Offset: last.end, Seq: last.event.Seq, Hash: last.hash}
	}
	return page, nil
}

func ReadableHasUpdates(path string, cursor ReadableCursor) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if cursor.Offset < 0 || cursor.Offset > info.Size() {
		return false, ErrInvalidReadableCursor
	}
	if err = validateReadableCursor(file, cursor); err != nil {
		return false, err
	}
	if cursor.Offset == info.Size() {
		return false, nil
	}
	if _, err = file.Seek(cursor.Offset, io.SeekStart); err != nil {
		return false, err
	}
	record, complete, readErr := readRecord(bufio.NewReaderSize(file, 64*1024), cursor.Offset)
	if !complete && errors.Is(readErr, io.EOF) {
		return false, nil
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, readErr
	}
	if !complete || record.event.Seq != cursor.Seq+1 {
		return false, ErrInvalidReadableCursor
	}
	return true, nil
}

func readRecord(reader io.Reader, offset int64) (readableRecord, bool, error) {
	buffered := bufio.NewReaderSize(reader, 64*1024)
	line, err := buffered.ReadBytes('\n')
	if len(line) > version.MaxAgentFrameBytes+1024*1024 {
		return readableRecord{}, false, fmt.Errorf("readable event record exceeds maximum size")
	}
	if len(line) == 0 {
		return readableRecord{}, false, err
	}
	if line[len(line)-1] != '\n' {
		return readableRecord{}, false, io.EOF
	}
	var event domain.Event
	if json.Unmarshal(bytesTrimNewline(line), &event) != nil || event.Seq == 0 {
		return readableRecord{}, false, fmt.Errorf("invalid readable event after byte %d", offset)
	}
	digest := sha256.Sum256(line)
	return readableRecord{event: event, start: offset, end: offset + int64(len(line)), hash: fmt.Sprintf("sha256:%x", digest)}, true, err
}

func validateReadableCursor(file *os.File, cursor ReadableCursor) error {
	if cursor.Offset == 0 && cursor.Seq == 0 && cursor.Hash == "" {
		return nil
	}
	if cursor.Offset <= 0 || cursor.Seq == 0 || cursor.Hash == "" {
		return ErrInvalidReadableCursor
	}
	record, err := readPreviousRecord(file, cursor.Offset)
	if err != nil || record.event.Seq != cursor.Seq || record.hash != cursor.Hash {
		return ErrInvalidReadableCursor
	}
	return nil
}

func readRecordsBefore(file *os.File, end int64, limit int) ([]readableRecord, error) {
	reversed := make([]readableRecord, 0, limit)
	for end > 0 && len(reversed) < limit {
		record, err := readPreviousRecord(file, end)
		if err != nil {
			return nil, err
		}
		reversed = append(reversed, record)
		end = record.start
	}
	records := make([]readableRecord, len(reversed))
	for index := range reversed {
		records[len(reversed)-1-index] = reversed[index]
	}
	for index := 1; index < len(records); index++ {
		if records[index].event.Seq != records[index-1].event.Seq+1 {
			return nil, fmt.Errorf("readable event sequence gap before %d", records[index].event.Seq)
		}
	}
	return records, nil
}

func readPreviousRecord(file *os.File, end int64) (readableRecord, error) {
	if end <= 0 {
		return readableRecord{}, io.EOF
	}
	var terminator [1]byte
	if _, err := file.ReadAt(terminator[:], end-1); err != nil || terminator[0] != '\n' {
		return readableRecord{}, ErrInvalidReadableCursor
	}
	start := int64(0)
	search := end - 1
	for search > 0 {
		size := int64(64 * 1024)
		if search < size {
			size = search
		}
		chunk := make([]byte, size)
		if _, err := file.ReadAt(chunk, search-size); err != nil {
			return readableRecord{}, err
		}
		if index := bytes.LastIndexByte(chunk, '\n'); index >= 0 {
			start = search - size + int64(index) + 1
			break
		}
		search -= size
		if end-search > version.MaxAgentFrameBytes+1024*1024 {
			return readableRecord{}, fmt.Errorf("readable event record exceeds maximum size")
		}
	}
	line := make([]byte, end-start)
	if _, err := file.ReadAt(line, start); err != nil {
		return readableRecord{}, err
	}
	var event domain.Event
	if json.Unmarshal(bytesTrimNewline(line), &event) != nil || event.Seq == 0 {
		return readableRecord{}, fmt.Errorf("invalid readable event after byte %d", start)
	}
	digest := sha256.Sum256(line)
	return readableRecord{event: event, start: start, end: end, hash: fmt.Sprintf("sha256:%x", digest)}, nil
}

func lastCompleteOffset(file *os.File, size int64) (int64, error) {
	if size == 0 {
		return 0, nil
	}
	var last [1]byte
	if _, err := file.ReadAt(last[:], size-1); err != nil {
		return 0, err
	}
	if last[0] == '\n' {
		return size, nil
	}
	search := size
	for search > 0 {
		chunkSize := int64(64 * 1024)
		if search < chunkSize {
			chunkSize = search
		}
		chunk := make([]byte, chunkSize)
		if _, err := file.ReadAt(chunk, search-chunkSize); err != nil {
			return 0, err
		}
		if index := bytes.LastIndexByte(chunk, '\n'); index >= 0 {
			return search - chunkSize + int64(index) + 1, nil
		}
		search -= chunkSize
		if size-search > version.MaxAgentFrameBytes+1024*1024 {
			return 0, fmt.Errorf("readable event record exceeds maximum size")
		}
	}
	return 0, nil
}
