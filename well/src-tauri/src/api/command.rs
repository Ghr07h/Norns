use serde::Deserialize;
use serde_json::{json, Value};
use tauri::State;

use crate::state::AppState;

use super::{
    client::{ApiClient, ViewResponse},
    dto::{
        AcceptedResult, CreateSubjectResult, CurrentRuntime, Event, EventPage, EventPageWire,
        EventUpdates, ProbeSnapshot, QueuePage, RuntimeDetail, RuntimePage, SubjectDetail,
        SubjectPage,
    },
    error::ClientError,
    url,
};

#[derive(Deserialize)]
pub struct ConnectionInput {
    pub base_url: String,
    pub password: String,
}

#[derive(Deserialize)]
pub struct PageInput {
    pub connection: ConnectionInput,
    pub cursor: Option<String>,
}

#[derive(Deserialize)]
pub struct SubjectInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
}

#[derive(Deserialize)]
pub struct RuntimeInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub runtime_id: String,
}

#[tauri::command]
pub fn normalize_server_url(input: String) -> Result<String, ClientError> {
    url::normalize_server_url(&input)
}

#[tauri::command]
pub async fn probe_server(
    state: State<'_, AppState>,
    connection: ConnectionInput,
) -> Result<ProbeSnapshot, ClientError> {
    api(&state, &connection).probe().await
}

#[tauri::command]
pub async fn list_subjects_page(
    state: State<'_, AppState>,
    input: PageInput,
) -> Result<SubjectPage, ClientError> {
    let mut query = vec![("limit", "200".into())];
    push_optional(&mut query, "cursor", input.cursor);
    api(&state, &input.connection)
        .get(&["subjects"], &query)
        .await
}

#[tauri::command]
pub async fn get_subject(
    state: State<'_, AppState>,
    input: SubjectInput,
) -> Result<SubjectDetail, ClientError> {
    api(&state, &input.connection)
        .get(&["subjects", &input.subject_id], &[])
        .await
}

#[derive(Deserialize)]
pub struct CreateSubjectInput {
    pub connection: ConnectionInput,
    pub name: Option<String>,
    pub instruction: Option<String>,
}

#[tauri::command]
pub async fn create_subject(
    state: State<'_, AppState>,
    input: CreateSubjectInput,
) -> Result<CreateSubjectResult, ClientError> {
    if let Some(instruction) = &input.instruction {
        validate_instruction(instruction)?;
    }
    let body = json!({
        "name": input.name,
        "runtime": input.instruction.map(|instruction| json!({"instruction": instruction})),
    });
    api(&state, &input.connection)
        .post(&["subjects"], &body)
        .await
}

#[derive(Deserialize)]
pub struct InstructionInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub instruction: String,
}

#[tauri::command]
pub async fn submit_instruction(
    state: State<'_, AppState>,
    input: InstructionInput,
) -> Result<AcceptedResult, ClientError> {
    validate_instruction(&input.instruction)?;
    api(&state, &input.connection)
        .post(
            &["subjects", &input.subject_id, "runtime"],
            &json!({"instruction": input.instruction}),
        )
        .await
}

#[tauri::command]
pub async fn get_current_runtime(
    state: State<'_, AppState>,
    input: SubjectInput,
) -> Result<CurrentRuntime, ClientError> {
    api(&state, &input.connection)
        .get(&["subjects", &input.subject_id, "runtime"], &[])
        .await
}

#[derive(Deserialize)]
pub struct StopInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub mode: String,
}

#[tauri::command]
pub async fn stop_runtime(
    state: State<'_, AppState>,
    input: StopInput,
) -> Result<AcceptedResult, ClientError> {
    if input.mode != "graceful" && input.mode != "force" {
        return Err(ClientError::protocol("stop mode must be graceful or force"));
    }
    api(&state, &input.connection)
        .post(
            &["subjects", &input.subject_id, "runtime", "stop"],
            &json!({"mode": input.mode}),
        )
        .await
}

#[tauri::command]
pub async fn list_queue_page(
    state: State<'_, AppState>,
    input: PageInput,
) -> Result<QueuePage, ClientError> {
    let mut query = vec![("limit", "200".into())];
    push_optional(&mut query, "cursor", input.cursor);
    api(&state, &input.connection).get(&["queue"], &query).await
}

#[derive(Deserialize)]
pub struct EventInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub after: Option<String>,
    pub cursor: Option<String>,
    pub limit: Option<u16>,
}

#[tauri::command]
pub async fn list_events(
    state: State<'_, AppState>,
    input: EventInput,
) -> Result<EventPage, ClientError> {
    if input.after.is_some() && input.cursor.is_some() {
        return Err(ClientError::protocol(
            "after and cursor are mutually exclusive",
        ));
    }
    let mut query = vec![(
        "limit",
        input.limit.unwrap_or(5000).clamp(1, 5000).to_string(),
    )];
    push_optional(&mut query, "after", input.after);
    push_optional(&mut query, "cursor", input.cursor);
    let page: EventPageWire = api(&state, &input.connection)
        .get(&["subjects", &input.subject_id, "events"], &query)
        .await?;
    Ok(EventPage {
        events: page.events.into_iter().map(Event::from).collect(),
        cursor: page.cursor,
        next_cursor: page.next_cursor,
        has_more: page.has_more,
    })
}

#[derive(Deserialize)]
pub struct EventUpdatesInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub cursor: String,
}

#[tauri::command]
pub async fn get_event_updates(
    state: State<'_, AppState>,
    input: EventUpdatesInput,
) -> Result<EventUpdates, ClientError> {
    api(&state, &input.connection)
        .get(
            &["subjects", &input.subject_id, "events", "updates"],
            &[("cursor", input.cursor)],
        )
        .await
}

#[derive(Deserialize)]
pub struct RuntimePageInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub cursor: Option<String>,
}

#[tauri::command]
pub async fn list_runtimes_page(
    state: State<'_, AppState>,
    input: RuntimePageInput,
) -> Result<RuntimePage, ClientError> {
    let mut query = vec![("limit", "200".into())];
    push_optional(&mut query, "cursor", input.cursor);
    api(&state, &input.connection)
        .get(&["subjects", &input.subject_id, "runtimes"], &query)
        .await
}

#[tauri::command]
pub async fn get_runtime(
    state: State<'_, AppState>,
    input: RuntimeInput,
) -> Result<RuntimeDetail, ClientError> {
    api(&state, &input.connection)
        .get(
            &["subjects", &input.subject_id, "runtimes", &input.runtime_id],
            &[],
        )
        .await
}

#[derive(Deserialize)]
pub struct StatusInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub task_id: Option<String>,
}

#[tauri::command]
pub async fn get_norns_status(
    state: State<'_, AppState>,
    input: StatusInput,
) -> Result<ViewResponse<Value>, ClientError> {
    let mut query = Vec::new();
    push_optional(&mut query, "task_id", input.task_id);
    api(&state, &input.connection)
        .get_view(
            &["subjects", &input.subject_id, "norns", "views", "status"],
            &query,
        )
        .await
}

#[derive(Deserialize)]
pub struct TraceInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub exploration_id: String,
}

#[derive(Deserialize)]
pub struct VisualInput {
    pub connection: ConnectionInput,
    pub subject_id: String,
    pub lang: String,
}

#[tauri::command]
pub async fn get_norns_trace(
    state: State<'_, AppState>,
    input: TraceInput,
) -> Result<ViewResponse<Value>, ClientError> {
    if input.exploration_id.trim().is_empty() {
        return Err(ClientError::protocol("exploration_id is required"));
    }
    api(&state, &input.connection)
        .get_view(
            &["subjects", &input.subject_id, "norns", "views", "trace"],
            &[("exploration_id", input.exploration_id)],
        )
        .await
}

#[tauri::command]
pub async fn get_norns_visual(
    state: State<'_, AppState>,
    input: VisualInput,
) -> Result<ViewResponse<String>, ClientError> {
    validate_visual_language(&input.lang)?;
    api(&state, &input.connection)
        .html(
            &["subjects", &input.subject_id, "norns", "views", "visual"],
            &[("lang", input.lang)],
        )
        .await
}

fn api<'a>(state: &'a State<'_, AppState>, input: &'a ConnectionInput) -> ApiClient<'a> {
    ApiClient::new(&state.client, &input.base_url, &input.password)
}

fn push_optional(
    query: &mut Vec<(&'static str, String)>,
    key: &'static str,
    value: Option<String>,
) {
    if let Some(value) = value {
        query.push((key, value));
    }
}

fn validate_instruction(value: &str) -> Result<(), ClientError> {
    if value.trim().is_empty() {
        return Err(ClientError::protocol(
            "instruction must contain non-whitespace characters",
        ));
    }
    if value.len() > 1_048_576 {
        return Err(ClientError::protocol("instruction exceeds the 1 MiB limit"));
    }
    Ok(())
}

fn validate_visual_language(value: &str) -> Result<(), ClientError> {
    if value != "zh" && value != "en" {
        return Err(ClientError::protocol("Visual language must be zh or en"));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn validates_command_scalars() {
        assert!(validate_instruction(" \n ").is_err());
        assert!(validate_instruction(&"x".repeat(1_048_577)).is_err());
        assert!(validate_instruction("  keep whitespace  ").is_ok());
        assert!(validate_visual_language("zh").is_ok());
        assert!(validate_visual_language("en").is_ok());
        assert!(validate_visual_language("de").is_err());
    }
}
