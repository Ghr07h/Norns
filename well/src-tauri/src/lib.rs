mod api;
mod state;

use api::command::*;
use state::AppState;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let state = AppState::new().expect("failed to initialize Well HTTP client");
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .manage(state)
        .invoke_handler(tauri::generate_handler![
            normalize_server_url,
            probe_server,
            list_subjects_page,
            get_subject,
            create_subject,
            submit_instruction,
            get_current_runtime,
            stop_runtime,
            list_queue_page,
            list_events,
            get_event_updates,
            list_runtimes_page,
            get_runtime,
            get_norns_status,
            get_norns_trace,
            get_norns_visual,
        ])
        .run(tauri::generate_context!())
        .expect("error while running Well");
}
