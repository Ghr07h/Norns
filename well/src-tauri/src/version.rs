include!(concat!(env!("OUT_DIR"), "/versions.rs"));

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn compiled_versions_match_root_manifest() {
        let manifest: serde_json::Value =
            serde_json::from_str(include_str!("../../../versions.json")).unwrap();
        assert_eq!(manifest["well"]["version"], env!("CARGO_PKG_VERSION"));
        assert_eq!(manifest["well"]["support_api_schema"], SUPPORT_API_SCHEMA);
    }
}
