use std::time::Duration;

use futures_util::StreamExt;
use reqwest::{header, Method, Response};
use serde::{de::DeserializeOwned, Serialize};
use url::Url;
use uuid::Uuid;

use super::{
    dto::{Health, ProbeSnapshot, ServiceInfo},
    error::ClientError,
    url::endpoint,
};

const JSON_LIMIT: usize = 128 * 1024 * 1024;
const HTML_LIMIT: usize = 64 * 1024 * 1024;
const PROBE_TIMEOUT: Duration = Duration::from_secs(3);

pub struct ApiClient<'a> {
    client: &'a reqwest::Client,
    base_url: &'a str,
    password: &'a str,
}

#[derive(Debug, Serialize)]
pub struct ViewResponse<T> {
    pub data: T,
    pub filename: Option<String>,
}

impl<'a> ApiClient<'a> {
    pub fn new(client: &'a reqwest::Client, base_url: &'a str, password: &'a str) -> Self {
        Self {
            client,
            base_url,
            password,
        }
    }

    pub async fn probe(&self) -> Result<ProbeSnapshot, ClientError> {
        self.probe_with_timeout(PROBE_TIMEOUT).await
    }

    async fn probe_with_timeout(&self, timeout: Duration) -> Result<ProbeSnapshot, ClientError> {
        tokio::time::timeout(timeout, self.probe_request())
            .await
            .map_err(|_| ClientError::new("timeout", "the nornsd connection attempt timed out"))?
    }

    async fn probe_request(&self) -> Result<ProbeSnapshot, ClientError> {
        let service: ServiceInfo = self.get(&[], &[]).await?;
        if service.service != "nornsd" || service.api_version != "v1" {
            return Err(ClientError::protocol(
                "server is not a supported nornsd API v1 service",
            ));
        }
        if let Some(server_id) = &service.server_id {
            if Uuid::parse_str(server_id).is_err() {
                return Err(ClientError::protocol(
                    "nornsd returned an invalid server_id",
                ));
            }
        }
        let health: Health = self.get(&["health"], &[]).await?;
        if health.status != "ok" {
            return Err(ClientError::protocol("nornsd health response is not ok"));
        }
        Ok(ProbeSnapshot { service, health })
    }

    pub async fn get<T: DeserializeOwned>(
        &self,
        segments: &[&str],
        query: &[(&str, String)],
    ) -> Result<T, ClientError> {
        Ok(self
            .json(Method::GET, segments, query, Option::<&()>::None)
            .await?
            .data)
    }

    pub async fn get_view<T: DeserializeOwned>(
        &self,
        segments: &[&str],
        query: &[(&str, String)],
    ) -> Result<ViewResponse<T>, ClientError> {
        self.json(Method::GET, segments, query, Option::<&()>::None)
            .await
    }

    pub async fn post<T: DeserializeOwned, B: Serialize + ?Sized>(
        &self,
        segments: &[&str],
        body: &B,
    ) -> Result<T, ClientError> {
        Ok(self
            .json(Method::POST, segments, &[], Some(body))
            .await?
            .data)
    }

    async fn json<T: DeserializeOwned, B: Serialize + ?Sized>(
        &self,
        method: Method,
        segments: &[&str],
        query: &[(&str, String)],
        body: Option<&B>,
    ) -> Result<ViewResponse<T>, ClientError> {
        let url = self.url(segments, query)?;
        let mut request = self
            .client
            .request(method, url)
            .bearer_auth(self.password)
            .timeout(Duration::from_secs(15));
        if let Some(value) = body {
            request = request.json(value);
        }
        let response = request.send().await.map_err(ClientError::from_reqwest)?;
        let status = response.status();
        let content_type = response
            .headers()
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok())
            .unwrap_or("")
            .to_ascii_lowercase();
        let filename = response
            .headers()
            .get("x-norns-view-filename")
            .and_then(|value| value.to_str().ok())
            .and_then(safe_view_filename);
        let body = read_limited(response, JSON_LIMIT).await?;
        if !status.is_success() {
            if status.as_u16() == 401 {
                return Err(ClientError::api(status.as_u16(), &body));
            }
            if !content_type.starts_with("application/json") {
                return Err(ClientError::protocol(
                    "nornsd error response returned an invalid Content-Type",
                ));
            }
            return Err(ClientError::api(status.as_u16(), &body));
        }
        if !content_type.starts_with("application/json") {
            return Err(ClientError::protocol(
                "nornsd JSON endpoint returned an invalid Content-Type",
            ));
        }
        let data = serde_json::from_slice(&body).map_err(|error| {
            ClientError::protocol(format!("invalid nornsd JSON response: {error}"))
        })?;
        Ok(ViewResponse { data, filename })
    }

    pub async fn html(
        &self,
        segments: &[&str],
        query: &[(&str, String)],
    ) -> Result<ViewResponse<String>, ClientError> {
        let url = self.url(segments, query)?;
        let response = self
            .client
            .get(url)
            .bearer_auth(self.password)
            .timeout(Duration::from_secs(30))
            .send()
            .await
            .map_err(ClientError::from_reqwest)?;
        let status = response.status();
        let content_type = response
            .headers()
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok())
            .unwrap_or("")
            .to_ascii_lowercase();
        let filename = response
            .headers()
            .get("x-norns-view-filename")
            .and_then(|value| value.to_str().ok())
            .and_then(safe_view_filename);
        let body = read_limited(response, HTML_LIMIT).await?;
        if !status.is_success() {
            if !content_type.starts_with("application/json") {
                return Err(ClientError::protocol(
                    "Norns Visual error response returned an invalid Content-Type",
                ));
            }
            return Err(ClientError::api(status.as_u16(), &body));
        }
        if !content_type.starts_with("text/html") {
            return Err(ClientError::protocol(
                "Norns Visual endpoint did not return HTML",
            ));
        }
        let data = String::from_utf8(body)
            .map_err(|_| ClientError::protocol("Norns Visual response is not valid UTF-8"))?;
        Ok(ViewResponse { data, filename })
    }

    fn url(&self, segments: &[&str], query: &[(&str, String)]) -> Result<Url, ClientError> {
        let mut url = endpoint(self.base_url, segments)?;
        if !query.is_empty() {
            let mut pairs = url.query_pairs_mut();
            for (key, value) in query {
                pairs.append_pair(key, value);
            }
        }
        Ok(url)
    }
}

fn safe_view_filename(value: &str) -> Option<String> {
    let value = value.trim();
    if value.is_empty()
        || value.len() > 255
        || value == "."
        || value == ".."
        || !value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
    {
        return None;
    }
    Some(value.to_owned())
}

async fn read_limited(response: Response, limit: usize) -> Result<Vec<u8>, ClientError> {
    if response
        .content_length()
        .is_some_and(|length| length > limit as u64)
    {
        return Err(ClientError::response_too_large(limit));
    }
    let mut stream = response.bytes_stream();
    let mut output = Vec::new();
    while let Some(chunk) = stream.next().await {
        let chunk = chunk.map_err(ClientError::from_reqwest)?;
        if output.len().saturating_add(chunk.len()) > limit {
            return Err(ClientError::response_too_large(limit));
        }
        output.extend_from_slice(&chunk);
    }
    Ok(output)
}

#[cfg(test)]
mod tests {
    use httpmock::prelude::*;
    use serde_json::{json, Value};

    use super::*;

    fn http_client() -> reqwest::Client {
        reqwest::Client::builder().build().unwrap()
    }

    fn service(server_id: Value) -> Value {
        json!({
            "service": "nornsd",
            "server_id": server_id,
            "version": "0.1.0",
            "api_version": "v1",
            "runtime_profile": { "agent": "cc", "agent_version": "1", "capabilities": [] },
            "protocol_versions": { "norns": null, "agent_config": 1 },
            "capacity": { "concurrency": 2, "using": 0, "queuing": 0 },
            "capabilities": []
        })
    }

    #[tokio::test]
    async fn probe_checks_auth_service_health_and_identity() {
        let server = MockServer::start();
        let root = server.mock(|when, then| {
            when.method(GET)
                .path("/api/v1")
                .header("authorization", "Bearer secret");
            then.status(200)
                .header("content-type", "application/json")
                .json_body(service(json!("550e8400-e29b-41d4-a716-446655440000")));
        });
        let health = server.mock(|when, then| {
            when.method(GET)
                .path("/api/v1/health")
                .header("authorization", "Bearer secret");
            then.status(200)
                .header("content-type", "application/json")
                .json_body(json!({ "status": "ok" }));
        });
        let client = http_client();
        let snapshot = ApiClient::new(&client, &server.base_url(), "secret")
            .probe()
            .await
            .unwrap();
        assert_eq!(snapshot.service.service, "nornsd");
        root.assert();
        health.assert();
    }

    #[tokio::test]
    async fn probe_applies_one_timeout_to_the_complete_attempt() {
        let server = MockServer::start();
        server.mock(|when, then| {
            when.method(GET).path("/api/v1");
            then.delay(Duration::from_millis(100))
                .status(200)
                .header("content-type", "application/json")
                .json_body(service(json!("550e8400-e29b-41d4-a716-446655440000")));
        });
        let client = http_client();
        let error = ApiClient::new(&client, &server.base_url(), "secret")
            .probe_with_timeout(Duration::from_millis(10))
            .await
            .err()
            .expect("probe should time out");
        assert_eq!(error.kind, "timeout");
    }

    #[tokio::test]
    async fn probe_accepts_missing_identity_and_rejects_invalid_identity() {
        let old = MockServer::start();
        old.mock(|when, then| {
            when.method(GET).path("/api/v1");
            let mut body = service(Value::Null);
            body.as_object_mut().unwrap().remove("server_id");
            then.status(200)
                .header("content-type", "application/json")
                .json_body(body);
        });
        old.mock(|when, then| {
            when.method(GET).path("/api/v1/health");
            then.status(200)
                .header("content-type", "application/json")
                .json_body(json!({ "status": "ok" }));
        });
        let client = http_client();
        assert!(ApiClient::new(&client, &old.base_url(), "secret")
            .probe()
            .await
            .is_ok());

        let invalid = MockServer::start();
        invalid.mock(|when, then| {
            when.method(GET).path("/api/v1");
            then.status(200)
                .header("content-type", "application/json")
                .json_body(service(json!("not-a-uuid")));
        });
        let error = ApiClient::new(&client, &invalid.base_url(), "secret")
            .probe()
            .await
            .err()
            .unwrap();
        assert_eq!(error.kind, "protocol");
    }

    #[tokio::test]
    async fn preserves_api_error_envelope_and_rejects_non_html_visual() {
        let server = MockServer::start();
        server.mock(|when, then| {
            when.method(GET).path("/api/v1/subjects");
            then.status(401).header("content-type", "application/json").json_body(json!({ "error": { "code": "UNAUTHORIZED", "message": "denied", "request_id": "req-1", "details": null } }));
        });
        server.mock(|when, then| {
            when.method(GET).path("/api/v1/visual");
            then.status(200)
                .header("content-type", "application/json")
                .body("{}");
        });
        let client = http_client();
        let base_url = server.base_url();
        let api = ApiClient::new(&client, &base_url, "secret");
        let error = api.get::<Value>(&["subjects"], &[]).await.unwrap_err();
        assert_eq!(error.kind, "authentication");
        assert_eq!(error.code.as_deref(), Some("UNAUTHORIZED"));
        assert_eq!(error.request_id.as_deref(), Some("req-1"));
        assert_eq!(
            api.html(&["visual"], &[]).await.unwrap_err().kind,
            "protocol"
        );
    }

    #[tokio::test]
    async fn view_responses_preserve_only_safe_server_filenames() {
        let server = MockServer::start();
        server.mock(|when, then| {
            when.method(GET).path("/api/v1/status");
            then.status(200)
                .header("content-type", "application/json")
                .header(
                    "x-norns-view-filename",
                    "status-20261022T080000.123456789Z-a1b2c3d4.json",
                )
                .json_body(json!({ "generated_at": "2026-10-22T08:00:00Z" }));
        });
        server.mock(|when, then| {
            when.method(GET).path("/api/v1/visual");
            then.status(200)
                .header("content-type", "text/html")
                .header("x-norns-view-filename", "../../escape.html")
                .body("<!doctype html><title>Visual</title>");
        });
        let client = http_client();
        let base_url = server.base_url();
        let api = ApiClient::new(&client, &base_url, "secret");

        let status = api.get_view::<Value>(&["status"], &[]).await.unwrap();
        assert_eq!(
            status.filename.as_deref(),
            Some("status-20261022T080000.123456789Z-a1b2c3d4.json")
        );
        let visual = api.html(&["visual"], &[]).await.unwrap();
        assert_eq!(visual.filename, None);
        assert!(visual.data.contains("Visual"));
    }

    #[tokio::test]
    async fn probe_classifies_any_unauthorized_response_as_authentication() {
        let server = MockServer::start();
        server.mock(|when, then| {
            when.method(GET).path("/api/v1");
            then.status(401)
                .header("content-type", "text/plain")
                .body("unauthorized");
        });
        let client = http_client();
        let error = ApiClient::new(&client, &server.base_url(), "wrong-token")
            .probe()
            .await
            .err()
            .expect("probe should reject an unauthorized response");
        assert_eq!(error.kind, "authentication");
        assert_eq!(error.http_status, Some(401));
    }

    #[tokio::test]
    async fn never_includes_password_in_configuration_errors() {
        let client = http_client();
        let error = ApiClient::new(&client, "not a url", "super-secret")
            .get::<Value>(&[], &[])
            .await
            .unwrap_err();
        assert!(!format!("{error:?}").contains("super-secret"));
        assert!(!error.to_string().contains("super-secret"));
    }
}
