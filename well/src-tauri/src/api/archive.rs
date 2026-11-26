use std::{path::Path, time::Duration};

use futures_util::StreamExt;
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter, State};
use tauri_plugin_dialog::DialogExt;
use tokio::{io::AsyncWriteExt, sync::oneshot};

use super::{command::ConnectionInput, error::ClientError, url::endpoint};
use crate::state::AppState;

#[derive(Deserialize)]
pub struct ArchiveInput {
    pub connection: ConnectionInput,
    pub request_id: String,
    pub scope: String,
    pub subject_id: Option<String>,
    pub runtime_id: Option<String>,
}

#[derive(Clone, Serialize)]
struct Progress<'a> {
    request_id: &'a str,
    received: u64,
    total: Option<u64>,
}

fn archive_query(input: &ArchiveInput) -> Result<Vec<(&'static str, String)>, ClientError> {
    if !["subject", "rundata"].contains(&input.scope.as_str())
        || input.runtime_id.is_some() && (input.scope != "rundata" || input.subject_id.is_none())
    {
        return Err(ClientError::protocol(
            "invalid archive scope or identifier combination",
        ));
    }
    for id in [&input.subject_id, &input.runtime_id].into_iter().flatten() {
        uuid::Uuid::parse_str(id)
            .map_err(|_| ClientError::protocol("invalid archive identifier"))?;
    }
    let mut query = vec![("scope", input.scope.clone())];
    if let Some(id) = &input.subject_id {
        query.push(("subject_id", id.clone()));
    }
    if let Some(id) = &input.runtime_id {
        query.push(("runtime_id", id.clone()));
    }
    Ok(query)
}

#[tauri::command]
pub async fn download_archive(
    app: AppHandle,
    state: State<'_, AppState>,
    input: ArchiveInput,
) -> Result<Option<String>, ClientError> {
    uuid::Uuid::parse_str(&input.request_id)
        .map_err(|_| ClientError::protocol("invalid download request ID"))?;
    let query = archive_query(&input)?;
    let (cancel_tx, cancel_rx) = oneshot::channel();
    {
        let mut downloads = state
            .downloads
            .lock()
            .map_err(|_| ClientError::protocol("download state unavailable"))?;
        if !downloads.is_empty() {
            return Err(ClientError::new(
                "api",
                "another download is already in progress",
            ));
        }
        downloads.insert(input.request_id.clone(), cancel_tx);
    }
    let work = async {
        let (sender, receiver) = oneshot::channel();
        app.dialog()
            .file()
            .add_filter("tar.gz", &["tar.gz"])
            .set_file_name(format!("well-{}-archive.tar.gz", input.scope))
            .save_file(move |file| {
                let _ = sender.send(file);
            });
        let selected = receiver
            .await
            .map_err(|_| ClientError::protocol("save dialog was closed"))?;
        let Some(file) = selected else {
            return Ok(None);
        };
        let path = file
            .into_path()
            .map_err(|e| ClientError::configuration("invalid archive destination", e))?;
        let mut url = endpoint(&input.connection.base_url, &["archive"])?;
        url.query_pairs_mut().extend_pairs(query);
        let request = state
            .client
            .get(url)
            .bearer_auth(&input.connection.password)
            .timeout(Duration::from_secs(600));
        download_to(request, &path, |received, total| {
            let _ = app.emit(
                "well:archive-progress",
                Progress {
                    request_id: &input.request_id,
                    received,
                    total,
                },
            );
        })
        .await?;
        Ok(Some(path.to_string_lossy().into_owned()))
    };
    let result = tokio::select! {
        result = work => result,
        _ = cancel_rx => Err(ClientError::new("cancelled", "download cancelled")),
    };
    if let Ok(mut downloads) = state.downloads.lock() {
        downloads.remove(&input.request_id);
    }
    result
}

#[tauri::command]
pub fn cancel_archive(state: State<'_, AppState>, request_id: String) -> Result<(), ClientError> {
    let sender = state
        .downloads
        .lock()
        .map_err(|_| ClientError::protocol("download state unavailable"))?
        .remove(&request_id);
    if let Some(sender) = sender {
        let _ = sender.send(());
    }
    Ok(())
}

// Keep the old destination intact until a complete response is on disk. The
// temporary file is removed on network errors, cancellation, or timeout.
async fn download_to(
    request: reqwest::RequestBuilder,
    path: &Path,
    progress: impl Fn(u64, Option<u64>),
) -> Result<(), ClientError> {
    let response = request.send().await.map_err(ClientError::from_reqwest)?;
    let status = response.status();
    if !status.is_success() {
        let mut body = Vec::new();
        let mut stream = response.bytes_stream();
        while let Some(chunk) = stream.next().await {
            let chunk = chunk.map_err(ClientError::from_reqwest)?;
            let remaining = 65536usize.saturating_sub(body.len());
            body.extend_from_slice(&chunk[..chunk.len().min(remaining)]);
            if body.len() >= 65536 {
                break;
            }
        }
        return Err(ClientError::api(status.as_u16(), &body));
    }
    let content_type = response
        .headers()
        .get(reqwest::header::CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");
    if content_type.split(';').next().unwrap_or("").trim() != "application/gzip" {
        return Err(ClientError::protocol(
            "archive endpoint did not return gzip",
        ));
    }
    let total = response.content_length();
    let temporary = tempfile::NamedTempFile::new_in(
        path.parent()
            .ok_or_else(|| ClientError::protocol("invalid archive destination"))?,
    )
    .map_err(|e| ClientError::configuration("cannot create archive file", e))?;
    let mut file = tokio::fs::File::from_std(
        temporary
            .reopen()
            .map_err(|e| ClientError::configuration("cannot open archive file", e))?,
    );
    let mut received = 0u64;
    let mut stream = response.bytes_stream();
    let mut last_update = std::time::Instant::now();
    progress(0, total);
    while let Some(chunk) = stream.next().await {
        let chunk = chunk.map_err(ClientError::from_reqwest)?;
        file.write_all(&chunk)
            .await
            .map_err(|e| ClientError::configuration("cannot write archive", e))?;
        received += chunk.len() as u64;
        if last_update.elapsed() >= Duration::from_millis(100) {
            progress(received, total);
            last_update = std::time::Instant::now();
        }
    }
    if total.is_some_and(|size| size != received) {
        return Err(ClientError::protocol("incomplete archive download"));
    }
    file.flush()
        .await
        .map_err(|e| ClientError::configuration("cannot flush archive", e))?;
    file.sync_all()
        .await
        .map_err(|e| ClientError::configuration("cannot sync archive", e))?;
    drop(file);
    temporary
        .persist(path)
        .map_err(|e| ClientError::configuration("cannot save archive", e))?;
    progress(received, total);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use httpmock::{Method::GET, MockServer};
    use std::io::{Read, Write};

    #[tokio::test]
    async fn streams_download_and_preserves_destination_on_api_failure() {
        let server = MockServer::start_async().await;
        let success = server
            .mock_async(|when, then| {
                when.method(GET)
                    .path("/api/archive")
                    .query_param("scope", "rundata")
                    .header("authorization", "Bearer test");
                then.status(200)
                    .header("content-type", "application/gzip")
                    .body([31, 139, 8, 0]);
            })
            .await;
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("archive.tar.gz");
        let client = reqwest::Client::new();
        download_to(
            client
                .get(server.url("/api/archive?scope=rundata"))
                .bearer_auth("test"),
            &path,
            |_, _| {},
        )
        .await
        .unwrap();
        assert_eq!(std::fs::read(&path).unwrap(), [31, 139, 8, 0]);
        success.assert_async().await;
        success.delete_async().await;
        server
            .mock_async(|when, then| {
                when.method(GET);
                then.status(409)
                    .header("content-type", "application/json")
                    .json_body(
                        serde_json::json!({"error":{"code":"ARCHIVE_BUSY","message":"packing"}}),
                    );
            })
            .await;
        let error = download_to(client.get(server.url("/api/archive")), &path, |_, _| {})
            .await
            .unwrap_err();
        assert_eq!(error.code.as_deref(), Some("ARCHIVE_BUSY"));
        assert_eq!(std::fs::read(&path).unwrap(), [31, 139, 8, 0]);
        assert_eq!(std::fs::read_dir(dir.path()).unwrap().count(), 1);
    }

    #[tokio::test]
    async fn cancelled_partial_download_removes_temporary_file_and_keeps_original() {
        let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        let (release_tx, release_rx) = std::sync::mpsc::channel::<()>();
        let worker = std::thread::spawn(move || {
            let (mut socket, _) = listener.accept().unwrap();
            socket
                .set_read_timeout(Some(Duration::from_secs(3)))
                .unwrap();
            let mut request = [0; 4096];
            let _ = socket.read(&mut request);
            let _ = socket.write_all(b"HTTP/1.1 200 OK\r\nContent-Type: application/gzip\r\nContent-Length: 999999\r\n\r\npartial");
            let _ = release_rx.recv_timeout(Duration::from_secs(3));
        });
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("existing.tar.gz");
        std::fs::write(&path, b"original").unwrap();
        let client = reqwest::Client::new();
        let result = tokio::time::timeout(
            Duration::from_millis(200),
            download_to(
                client.get(format!("http://{address}/api/archive")),
                &path,
                |_, _| {},
            ),
        )
        .await;
        assert!(result.is_err());
        assert_eq!(std::fs::read(&path).unwrap(), b"original");
        assert_eq!(std::fs::read_dir(dir.path()).unwrap().count(), 1);
        let _ = release_tx.send(());
        worker.join().unwrap();
    }

    #[test]
    fn validates_five_scopes() {
        let mut input = ArchiveInput {
            connection: ConnectionInput {
                base_url: "http://localhost".into(),
                password: "test".into(),
            },
            request_id: "ignored".into(),
            scope: "subject".into(),
            subject_id: None,
            runtime_id: None,
        };
        assert!(archive_query(&input).is_ok());
        input.scope = "rundata".into();
        assert!(archive_query(&input).is_ok());
        input.subject_id = Some("11111111-1111-4111-8111-111111111111".into());
        assert!(archive_query(&input).is_ok());
        input.runtime_id = Some("22222222-2222-4222-8222-222222222222".into());
        assert!(archive_query(&input).is_ok());
        input.scope = "subject".into();
        assert!(archive_query(&input).is_err());
        input.runtime_id = None;
        assert!(archive_query(&input).is_ok());
        input.scope = "rundata".into();
        input.subject_id = None;
        input.runtime_id = Some("22222222-2222-4222-8222-222222222222".into());
        assert!(archive_query(&input).is_err());
    }
}
