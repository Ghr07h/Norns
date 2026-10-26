use url::Url;

use super::error::ClientError;

pub fn normalize_server_url(input: &str) -> Result<String, ClientError> {
    let mut url = Url::parse(input.trim())
        .map_err(|error| ClientError::configuration("invalid server URL", error))?;
    if url.scheme() != "http" && url.scheme() != "https" {
        return Err(ClientError::protocol("server URL must use http or https"));
    }
    if url.host_str().is_none()
        || !url.username().is_empty()
        || url.password().is_some()
        || url.query().is_some()
        || url.fragment().is_some()
        || !matches!(url.path(), "" | "/")
    {
        return Err(ClientError::protocol(
            "server URL must be an origin without credentials, path, query, or fragment",
        ));
    }
    if (url.scheme() == "http" && url.port() == Some(80))
        || (url.scheme() == "https" && url.port() == Some(443))
    {
        url.set_port(None)
            .map_err(|_| ClientError::protocol("invalid server port"))?;
    }
    Ok(url.as_str().trim_end_matches('/').to_string())
}

pub fn endpoint(base_url: &str, segments: &[&str]) -> Result<Url, ClientError> {
    let canonical = normalize_server_url(base_url)?;
    let mut url = Url::parse(&canonical)
        .map_err(|error| ClientError::configuration("invalid canonical URL", error))?;
    {
        let mut path = url
            .path_segments_mut()
            .map_err(|_| ClientError::protocol("server URL cannot be a base URL"))?;
        path.clear();
        path.push("api");
        path.push("v1");
        for segment in segments {
            path.push(segment);
        }
    }
    Ok(url)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn normalizes_origins_and_encodes_segments() {
        assert_eq!(
            normalize_server_url(" HTTP://Example.COM:80/ ").unwrap(),
            "http://example.com"
        );
        assert_eq!(
            endpoint("https://example.com", &["subjects", "a/b"])
                .unwrap()
                .as_str(),
            "https://example.com/api/v1/subjects/a%2Fb"
        );
        assert!(normalize_server_url("https://user@example.com/path").is_err());
    }
}
