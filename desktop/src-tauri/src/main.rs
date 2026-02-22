#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use tauri::Manager;
use tauri_plugin_shell::ShellExt;

const PORT: u16 = 41295;
const HEALTH_URL: &str = "http://127.0.0.1:41295/api/v1/system/health";
const POLL_INTERVAL_MS: u64 = 500;
const MAX_WAIT_SECS: u64 = 30;

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            let handle = app.handle().clone();

            // Resolve platform-appropriate data directory
            let data_dir = app
                .path()
                .app_data_dir()
                .expect("failed to resolve app data dir");
            std::fs::create_dir_all(&data_dir).expect("failed to create data dir");

            let data_dir_str = data_dir.to_string_lossy().to_string();

            // Spawn the Go sidecar
            let sidecar = handle
                .shell()
                .sidecar("openpaw")
                .expect("failed to locate openpaw sidecar")
                .env("OPENPAW_DATA_DIR", &data_dir_str)
                .env("OPENPAW_NO_OPEN", "1")
                .env("OPENPAW_BIND", "127.0.0.1");

            let (mut _rx, child) = sidecar.spawn().expect("failed to spawn openpaw sidecar");

            // Store child process for cleanup
            app.manage(SidecarState {
                child: std::sync::Mutex::new(Some(child)),
            });

            // Poll health endpoint, then navigate webview
            let handle_clone = handle.clone();
            tauri::async_runtime::spawn(async move {
                let client = reqwest::Client::new();
                let max_attempts = (MAX_WAIT_SECS * 1000) / POLL_INTERVAL_MS;

                for _ in 0..max_attempts {
                    tokio::time::sleep(std::time::Duration::from_millis(POLL_INTERVAL_MS)).await;
                    if let Ok(resp) = client.get(HEALTH_URL).send().await {
                        if resp.status().is_success() {
                            // Server is ready — navigate and show window
                            let url = format!("http://127.0.0.1:{}", PORT);
                            if let Some(window) = handle_clone.get_webview_window("main") {
                                let _ = window.navigate(url.parse().unwrap());
                                // Small delay for the page to start loading
                                tokio::time::sleep(std::time::Duration::from_millis(300)).await;
                                let _ = window.show();
                                let _ = window.set_focus();
                            }
                            return;
                        }
                    }
                }
                eprintln!("OpenPaw server did not start within {} seconds", MAX_WAIT_SECS);
            });

            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                // Kill sidecar on window close
                if let Some(state) = window.try_state::<SidecarState>() {
                    if let Ok(mut guard) = state.child.lock() {
                        if let Some(child) = guard.take() {
                            let _ = child.kill();
                        }
                    }
                }
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running OpenPaw desktop");
}

struct SidecarState {
    child: std::sync::Mutex<Option<tauri_plugin_shell::process::CommandChild>>,
}
