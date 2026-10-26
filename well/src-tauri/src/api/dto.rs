use serde::{Deserialize, Deserializer, Serialize, Serializer};
use serde_json::Value;

#[derive(Clone, Serialize, Deserialize)]
pub struct RuntimeProfile {
    pub agent: String,
    pub agent_version: String,
    #[serde(default)]
    pub capabilities: Vec<String>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct Capacity {
    pub concurrency: u32,
    pub using: u32,
    pub queuing: u32,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct ServiceInfo {
    pub service: String,
    #[serde(default)]
    pub server_id: Option<String>,
    pub version: String,
    pub api_version: String,
    pub runtime_profile: RuntimeProfile,
    pub protocol_versions: Value,
    pub capacity: Capacity,
    #[serde(default)]
    pub capabilities: Vec<String>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct Health {
    pub status: String,
}

#[derive(Clone, Serialize)]
pub struct ProbeSnapshot {
    pub service: ServiceInfo,
    pub health: Health,
}

#[derive(Clone, Default)]
pub enum MaybeField<T> {
    #[default]
    Missing,
    Null,
    Value(T),
}

impl<T> MaybeField<T> {
    pub fn is_missing(&self) -> bool {
        matches!(self, Self::Missing)
    }
}

impl<'de, T: Deserialize<'de>> Deserialize<'de> for MaybeField<T> {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        Option::<T>::deserialize(deserializer).map(|value| match value {
            Some(value) => Self::Value(value),
            None => Self::Null,
        })
    }
}

impl<T: Serialize> Serialize for MaybeField<T> {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        match self {
            Self::Value(value) => value.serialize(serializer),
            Self::Null | Self::Missing => serializer.serialize_none(),
        }
    }
}

#[derive(Clone, Serialize, Deserialize)]
pub struct Instruction {
    pub sequence: u64,
    pub content: String,
    pub created_at: String,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct Runtime {
    pub id: String,
    #[serde(default)]
    pub subject_id: Option<String>,
    pub state: String,
    #[serde(default)]
    pub queue_position: Option<u64>,
    #[serde(default)]
    pub instructions: Vec<Instruction>,
    pub created_at: String,
    #[serde(default)]
    pub queued_at: Option<String>,
    #[serde(default)]
    pub started_at: Option<String>,
    #[serde(default)]
    pub ended_at: Option<String>,
    #[serde(default)]
    pub resources: Value,
    #[serde(default)]
    pub closing: bool,
    #[serde(default)]
    pub close_deadline: Option<String>,
    pub state_changed_at: String,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct Workspace {
    pub materialized: bool,
    pub directory_name: String,
    #[serde(default, skip_serializing_if = "MaybeField::is_missing")]
    pub path: MaybeField<String>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct SubjectSummary {
    pub id: String,
    pub name: Option<String>,
    pub state: String,
    pub created_at: String,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct SubjectDetail {
    pub id: String,
    pub name: Option<String>,
    pub state: String,
    pub runtime_profile: RuntimeProfile,
    pub workspace: Workspace,
    pub last_event_seq: u64,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct SubjectPage {
    pub subjects: Vec<SubjectSummary>,
    pub next_cursor: Option<String>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct QueueEntry {
    pub position: u64,
    pub runtime_id: String,
    pub subject_id: String,
    pub subject_name: Option<String>,
    pub instruction_count: u64,
    pub queued_at: String,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct QueuePage {
    pub runtimes: Vec<QueueEntry>,
    pub next_cursor: Option<String>,
}

#[derive(Clone, Deserialize)]
pub struct EventWire {
    pub seq: u64,
    #[serde(rename = "type")]
    pub event_type: String,
    pub timestamp: String,
    pub runtime_id: Option<String>,
    pub source: String,
    pub payload: Value,
}

#[derive(Clone, Serialize)]
pub struct Event {
    pub seq: String,
    #[serde(rename = "type")]
    pub event_type: String,
    pub timestamp: String,
    pub runtime_id: Option<String>,
    pub source: String,
    pub payload: Value,
}

impl From<EventWire> for Event {
    fn from(value: EventWire) -> Self {
        Self {
            seq: value.seq.to_string(),
            event_type: value.event_type,
            timestamp: value.timestamp,
            runtime_id: value.runtime_id,
            source: value.source,
            payload: value.payload,
        }
    }
}

#[derive(Deserialize)]
pub struct EventPageWire {
    pub events: Vec<EventWire>,
    pub cursor: Option<String>,
    pub next_cursor: Option<String>,
    pub has_more: bool,
}

#[derive(Serialize)]
pub struct EventPage {
    pub events: Vec<Event>,
    pub cursor: Option<String>,
    pub next_cursor: Option<String>,
    pub has_more: bool,
}

#[derive(Serialize, Deserialize)]
pub struct EventUpdates {
    pub has_updates: bool,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct RuntimeHistorySummary {
    pub id: String,
    pub state: String,
    pub current: bool,
    pub created_at: String,
    #[serde(default)]
    pub started_at: Option<String>,
    pub ended_at: Option<String>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct RuntimePage {
    pub runtimes: Vec<RuntimeHistorySummary>,
    pub next_cursor: Option<String>,
    #[serde(default)]
    pub total_count: u64,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct RuntimeArtifacts {
    #[serde(default, skip_serializing_if = "MaybeField::is_missing")]
    pub raw_log_path: MaybeField<String>,
    #[serde(default, skip_serializing_if = "MaybeField::is_missing")]
    pub readable_log_path: MaybeField<String>,
    #[serde(default, skip_serializing_if = "MaybeField::is_missing")]
    pub transcript_path: MaybeField<String>,
    #[serde(default, skip_serializing_if = "MaybeField::is_missing")]
    pub transcript_status: MaybeField<String>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct RuntimeDetail {
    pub id: String,
    pub subject_id: String,
    pub state: String,
    pub agent: String,
    pub session_id: String,
    pub queued_at: Option<String>,
    pub started_at: Option<String>,
    pub ended_at: Option<String>,
    pub end_reason: Option<String>,
    pub settings_slot: Option<u32>,
    pub config_hash: Option<String>,
    pub process: Value,
    pub usage: Value,
    pub resources: Value,
    pub error: Value,
    #[serde(default, skip_serializing_if = "MaybeField::is_missing")]
    pub artifacts: MaybeField<RuntimeArtifacts>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct CurrentRuntime {
    pub runtime: Option<Runtime>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct CreateSubjectResult {
    pub subject_id: String,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct AcceptedResult {
    pub accepted: bool,
    #[serde(default)]
    pub runtime: Value,
    #[serde(default)]
    pub subject_id: Option<String>,
    #[serde(default)]
    pub runtime_id: Option<String>,
    #[serde(default)]
    pub runtime_state: Option<String>,
}
