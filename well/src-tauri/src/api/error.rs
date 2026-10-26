use serde::Serialize;
use serde_json::{Map, Value};

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ClientError {
    pub kind: &'static str,
    pub http_status: Option<u16>,
    pub code: Option<String>,
    pub message: String,
    pub request_id: Option<String>,
    pub details: Map<String, Value>,
}

impl ClientError {
    pub fn new(kind: &'static str, message: impl Into<String>) -> Self {
        Self {
            kind,
            http_status: None,
            code: None,
            message: message.into(),
            request_id: None,
            details: Map::new(),
        }
    }

    pub fn configuration(context: &str, error: impl std::fmt::Display) -> Self {
        Self::new("configuration", format!("{context}: {error}"))
    }

    pub fn protocol(message: impl Into<String>) -> Self {
        Self::new("protocol", message)
    }

    pub fn from_reqwest(error: reqwest::Error) -> Self {
        if error.is_timeout() {
            return Self::new("timeout", "the nornsd request timed out");
        }
        Self::new("connection", format!("cannot reach nornsd: {error}"))
    }

    pub fn response_too_large(limit: usize) -> Self {
        let mut error = Self::new(
            "response_too_large",
            "nornsd response exceeds the client limit",
        );
        error
            .details
            .insert("limit".into(), Value::from(limit as u64));
        error
    }

    pub fn api(status: u16, body: &[u8]) -> Self {
        #[derive(serde::Deserialize)]
        struct Envelope {
            error: ApiError,
        }
        #[derive(serde::Deserialize)]
        struct ApiError {
            code: Option<String>,
            message: Option<String>,
            request_id: Option<String>,
            #[serde(default)]
            details: Option<Map<String, Value>>,
        }
        let parsed = serde_json::from_slice::<Envelope>(body).ok();
        let kind = if status == 401 {
            "authentication"
        } else {
            "api"
        };
        let mut error = Self::new(
            kind,
            parsed
                .as_ref()
                .and_then(|value| value.error.message.clone())
                .unwrap_or_else(|| format!("nornsd returned HTTP {status}")),
        );
        error.http_status = Some(status);
        if let Some(envelope) = parsed {
            error.code = envelope.error.code;
            error.request_id = envelope.error.request_id;
            error.details = envelope.error.details.unwrap_or_default();
        }
        error
    }
}

impl std::fmt::Debug for ClientError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ClientError")
            .field("kind", &self.kind)
            .field("http_status", &self.http_status)
            .field("code", &self.code)
            .field("message", &self.message)
            .field("request_id", &self.request_id)
            .field("details", &self.details)
            .finish()
    }
}

impl std::fmt::Display for ClientError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str(&self.message)
    }
}

impl std::error::Error for ClientError {}
