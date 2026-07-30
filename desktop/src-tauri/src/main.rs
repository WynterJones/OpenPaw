#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use tauri::{Manager, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_shell::ShellExt;

const PORT: u16 = 41295;
const HEALTH_URL: &str = "http://127.0.0.1:41295/api/v1/system/health";
const POLL_INTERVAL_MS: u64 = 500;
const MAX_WAIT_SECS: u64 = 30;

/// How long the splash stays up even when the sidecar is ready sooner. Without
/// a floor it flashes past on a warm start, which reads as a glitch rather than
/// an intro.
const SPLASH_MIN_MS: u128 = 2800;

/// Time allowed for the splash's own fade-out before the window is destroyed.
/// Must stay in step with the .stage transition in splash.html.
const SPLASH_EXIT_MS: u64 = 420;

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        // Remembers the main window's size and position across launches.
        .plugin(
            tauri_plugin_window_state::Builder::default()
                .with_state_flags(
                    tauri_plugin_window_state::StateFlags::SIZE
                        | tauri_plugin_window_state::StateFlags::POSITION
                        | tauri_plugin_window_state::StateFlags::MAXIMIZED,
                )
                // The splash is transient and centred on every launch; saving
                // its geometry would restore a stale position next time.
                .with_denylist(&["splash"])
                .build(),
        )
        .setup(|app| {
            let handle = app.handle().clone();
            // Whether this is a first run has to be answered before the window
            // is shown, because the plugin has already restored any saved
            // geometry by then.
            let first_run = !window_state_saved(app);

            // The splash is created before anything slow happens, so it is on
            // screen while the sidecar boots rather than after.
            //
            // Built here rather than declared in tauri.conf.json because only
            // the programmatic builder takes an initialization script, which is
            // how the version reaches the page. The desktop capability is scoped
            // to the "main" window, so the splash has no Tauri permissions —
            // injected JS needs none.
            let version = app.package_info().version.to_string();
            let splash =
                WebviewWindowBuilder::new(&handle, "splash", WebviewUrl::App("splash.html".into()))
                    .title("OpenPaw")
                    .inner_size(460.0, 320.0)
                    .resizable(false)
                    .decorations(false)
                    .always_on_top(true)
                    .skip_taskbar(true)
                    .center()
                    .initialization_script(&format!(
                        "window.__OPENPAW_VERSION__ = {};",
                        serde_json::to_string(&version).unwrap_or_else(|_| "\"\"".into())
                    ))
                    .build();

            if let Err(e) = &splash {
                // A splash that won't build must not stop the app launching.
                eprintln!("failed to create splash window: {e}");
            }

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
                .env("OPENPAW_NO_OPEN", "1");
            // OPENPAW_BIND is deliberately NOT set. It used to be pinned to
            // 127.0.0.1 here, which silently overrode the Remote Access setting
            // and made it impossible to reach the desktop app from a phone.
            // The server now decides for itself and always keeps a loopback
            // listener, so this window still reaches its backend either way.

            let (mut _rx, child) = sidecar.spawn().expect("failed to spawn openpaw sidecar");

            // Store child process for cleanup
            app.manage(SidecarState {
                child: std::sync::Mutex::new(Some(child)),
            });

            // Poll health endpoint, then navigate webview
            let handle_clone = handle.clone();
            let started = std::time::Instant::now();
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

                                hold_splash(started).await;

                                // Full size on a first run only. After that the
                                // window-state plugin has already restored
                                // whatever size the user last chose, and
                                // maximizing would throw it away every launch.
                                if first_run {
                                    let _ = window.maximize();
                                }

                                // Main is revealed behind the always-on-top
                                // splash first, so the handoff never exposes an
                                // empty desktop between the two windows.
                                let _ = window.show();
                                let _ = window.set_focus();
                            }

                            dismiss_splash(&handle_clone).await;
                            return;
                        }
                    }
                }

                eprintln!(
                    "OpenPaw server did not start within {} seconds",
                    MAX_WAIT_SECS
                );
                // Leaving the splash up forever would look like a hang with no
                // way out, so clear it and let the main window surface whatever
                // error it can.
                dismiss_splash(&handle_clone).await;
                if let Some(window) = handle_clone.get_webview_window("main") {
                    let _ = window.show();
                }
            });

            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                // Scoped to the main window: the splash is destroyed during a
                // normal launch, and killing the sidecar there would take the
                // backend down moments after it finished starting.
                if window.label() != "main" {
                    return;
                }
                if let Some(state) = window.try_state::<SidecarState>() {
                    if let Ok(mut guard) = state.child.lock() {
                        if let Some(child) = guard.take() {
                            terminate_sidecar(child);
                        }
                    }
                }
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running OpenPaw desktop");
}

/// Reports whether the window-state plugin has a saved layout yet, i.e. whether
/// the user has ever sized this window themselves.
fn window_state_saved(app: &tauri::App) -> bool {
    app.path()
        .app_config_dir()
        .map(|dir| {
            dir.join(tauri_plugin_window_state::DEFAULT_FILENAME)
                .exists()
        })
        .unwrap_or(false)
}

/// Keeps the splash on screen for at least SPLASH_MIN_MS from launch.
async fn hold_splash(started: std::time::Instant) {
    let elapsed = started.elapsed().as_millis();
    if elapsed < SPLASH_MIN_MS {
        let remaining = (SPLASH_MIN_MS - elapsed) as u64;
        tokio::time::sleep(std::time::Duration::from_millis(remaining)).await;
    }
}

/// Plays the splash's fade-out, then closes it.
async fn dismiss_splash(handle: &tauri::AppHandle) {
    let Some(splash) = handle.get_webview_window("splash") else {
        return;
    };
    // Ignored on failure: if the eval doesn't land the window still closes, it
    // just cuts rather than fades.
    let _ = splash.eval("document.body.classList.add('leaving')");
    tokio::time::sleep(std::time::Duration::from_millis(SPLASH_EXIT_MS)).await;
    let _ = splash.close();
}

struct SidecarState {
    child: std::sync::Mutex<Option<tauri_plugin_shell::process::CommandChild>>,
}

#[cfg(unix)]
fn terminate_sidecar(child: tauri_plugin_shell::process::CommandChild) {
    // CommandChild::kill sends SIGKILL on Unix. That prevented the Go backend
    // from stopping its service children, leaving their ports occupied on the
    // next app launch. SIGTERM enters the backend's normal shutdown path.
    let result = unsafe { libc::kill(child.pid() as libc::pid_t, libc::SIGTERM) };
    if result != 0 {
        let _ = child.kill();
    }
}

#[cfg(not(unix))]
fn terminate_sidecar(child: tauri_plugin_shell::process::CommandChild) {
    let _ = child.kill();
}
