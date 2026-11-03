use std::{env, fs, path::PathBuf};

fn main() {
    let manifest_path = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../versions.json");
    println!("cargo:rerun-if-changed={}", manifest_path.display());
    let manifest: serde_json::Value = serde_json::from_str(
        &fs::read_to_string(&manifest_path).expect("failed to read root versions.json"),
    )
    .expect("invalid root versions.json");
    let version = manifest["well"]["version"]
        .as_str()
        .expect("versions.json: well.version must be a string");
    assert_eq!(
        version,
        env!("CARGO_PKG_VERSION"),
        "Well version metadata is stale; run npm run prepare:versions in well before Cargo commands"
    );
    let schema = manifest["well"]["support_api_schema"]
        .as_u64()
        .filter(|schema| *schema > 0 && *schema <= 9_007_199_254_740_991)
        .expect("versions.json: well.support_api_schema must be a positive safe integer");
    let output = PathBuf::from(env::var_os("OUT_DIR").expect("missing OUT_DIR"));
    fs::write(
        output.join("versions.rs"),
        format!("pub const SUPPORT_API_SCHEMA: u64 = {schema};\n"),
    )
    .expect("failed to generate Well API schema constant");
    tauri_build::build()
}
