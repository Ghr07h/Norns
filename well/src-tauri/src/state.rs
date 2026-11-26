use std::{collections::HashMap, sync::Mutex, time::Duration};
use tokio::sync::oneshot;

use reqwest::Client;

use crate::api::error::ClientError;

pub struct AppState {
    pub client: Client,
    pub downloads: Mutex<HashMap<String, oneshot::Sender<()>>>,
}

impl AppState {
    pub fn new() -> Result<Self, ClientError> {
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(8))
            .pool_idle_timeout(Duration::from_secs(90))
            .build()
            .map_err(|error| ClientError::configuration("cannot build HTTP client", error))?;
        Ok(Self {
            client,
            downloads: Mutex::new(HashMap::new()),
        })
    }
}
