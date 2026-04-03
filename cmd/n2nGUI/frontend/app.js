const state = {
  lastLogID: 0,
  toastTimer: null,
};

const els = {
  form: document.getElementById("config-form"),
  addressRow: document.getElementById("address-row"),
  statusPill: document.getElementById("status-pill"),
  platformLabel: document.getElementById("platform-label"),
  serverAddress: document.getElementById("server-address"),
  metricRunning: document.getElementById("metric-running"),
  metricRunningCopy: document.getElementById("metric-running-copy"),
  metricSupernode: document.getElementById("metric-supernode"),
  metricError: document.getElementById("metric-error"),
  diagnostics: document.getElementById("diagnostics"),
  logView: document.getElementById("log-view"),
  toast: document.getElementById("toast"),
  saveButton: document.getElementById("save-button"),
  startButton: document.getElementById("start-button"),
  stopButton: document.getElementById("stop-button"),
  clearLogsButton: document.getElementById("clear-logs-button"),
  exportLogsButton: document.getElementById("export-logs-button"),
};

function showToast(message) {
  els.toast.hidden = false;
  els.toast.textContent = message;
  clearTimeout(state.toastTimer);
  state.toastTimer = window.setTimeout(() => {
    els.toast.hidden = true;
  }, 2600);
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || `Request failed: ${response.status}`);
  }
  return payload;
}

function applyConfig(config) {
  els.form.community.value = config.community || "";
  els.form.addressMode.value = config.addressMode || "dhcp";
  els.form.address.value = config.address || "";
  els.form.supernodeHost.value = config.supernodeHost || "";
  els.form.supernodePort.value = config.supernodePort || 7654;
  els.form.mtu.value = config.mtu || 1300;
  els.form.extraArgs.value = config.extraArgs || "";
  updateAddressVisibility();
  els.metricSupernode.textContent = `${config.supernodeHost}:${config.supernodePort}`;
}

function readConfig() {
  return {
    community: els.form.community.value.trim(),
    addressMode: els.form.addressMode.value,
    address: els.form.address.value.trim(),
    supernodeHost: els.form.supernodeHost.value.trim(),
    supernodePort: Number(els.form.supernodePort.value),
    mtu: Number(els.form.mtu.value),
    extraArgs: els.form.extraArgs.value.trim(),
  };
}

function updateAddressVisibility() {
  const isStatic = els.form.addressMode.value === "static";
  els.addressRow.hidden = !isStatic;
}

function renderStatus(status) {
  const running = Boolean(status.running);
  els.statusPill.textContent = running ? "Edge running" : "Edge stopped";
  els.statusPill.classList.toggle("running", running);
  els.statusPill.classList.toggle("stopped", !running);
  els.platformLabel.textContent = status.platform || "Unknown";
  els.serverAddress.textContent = window.location.host;
  els.metricRunning.textContent = running ? "Running" : "Stopped";
  els.metricRunningCopy.textContent = running
    ? `PID ${status.pid} active`
    : status.binaryFound
      ? "Binary detected and ready to start."
      : "Binary not found under the current n2n directory.";
  els.metricError.textContent = status.lastError || "None";
}

function renderDiagnostics(diagnostics) {
  const rows = [
    ["Platform", diagnostics.platform || "-"],
    ["Binary Path", diagnostics.binaryPath || "-"],
    ["Binary Found", diagnostics.binaryFound ? "yes" : "no"],
    ["Config Path", diagnostics.configPath || "-"],
    ["Legacy INI", diagnostics.legacyIniPath || "-"],
    ["Working Dir", diagnostics.workingDir || "-"],
  ];

  els.diagnostics.innerHTML = rows
    .map(
      ([label, value]) =>
        `<div><dt>${label}</dt><dd>${escapeHTML(String(value))}</dd></div>`,
    )
    .join("");
}

function renderLogs(entries) {
  if (!entries.length && state.lastLogID === 0) {
    els.logView.textContent = "No logs yet.";
    return;
  }

  if (state.lastLogID === 0) {
    els.logView.textContent = "";
  }

  for (const entry of entries) {
    state.lastLogID = Math.max(state.lastLogID, entry.id);
    const line = `[${new Date(entry.timestamp).toLocaleTimeString()}] ${entry.stream.toUpperCase()} ${entry.message}\n`;
    els.logView.textContent += line;
  }
  els.logView.scrollTop = els.logView.scrollHeight;
}

function escapeHTML(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function refreshConfig() {
  const config = await api("/api/config");
  applyConfig(config);
}

async function refreshStatus() {
  const status = await api("/api/status");
  renderStatus(status);
}

async function refreshDiagnostics() {
  const diagnostics = await api("/api/diagnostics");
  renderDiagnostics(diagnostics);
}

async function refreshLogs() {
  const entries = await api(`/api/logs?since=${state.lastLogID}`);
  renderLogs(entries);
}

async function clearLogs() {
  await api("/api/logs", { method: "DELETE" });
  state.lastLogID = 0;
  els.logView.textContent = "No logs yet.";
  showToast("Log buffer cleared");
}

async function exportLogs() {
  const response = await fetch("/api/logs/export");
  if (!response.ok) {
    throw new Error(`Export failed: ${response.status}`);
  }

  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "n2n-edge.log";
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
  showToast("Logs exported");
}

async function saveConfig() {
  const config = readConfig();
  await api("/api/config", {
    method: "PUT",
    body: JSON.stringify(config),
  });
  applyConfig(config);
  showToast("Configuration saved");
}

async function startEdge() {
  await api("/api/control/start", { method: "POST" });
  await refreshStatus();
  showToast("Edge process started");
}

async function stopEdge() {
  await api("/api/control/stop", { method: "POST" });
  await refreshStatus();
  showToast("Edge process stopped");
}

function bindEvents() {
  els.form.addressMode.addEventListener("change", updateAddressVisibility);
  els.saveButton.addEventListener("click", () =>
    saveConfig().catch((error) => showToast(error.message)),
  );
  els.startButton.addEventListener("click", () =>
    startEdge().catch((error) => showToast(error.message)),
  );
  els.stopButton.addEventListener("click", () =>
    stopEdge().catch((error) => showToast(error.message)),
  );
  els.clearLogsButton.addEventListener("click", () =>
    clearLogs().catch((error) => showToast(error.message)),
  );
  els.exportLogsButton.addEventListener("click", () =>
    exportLogs().catch((error) => showToast(error.message)),
  );
}

async function boot() {
  bindEvents();
  try {
    await Promise.all([refreshConfig(), refreshStatus(), refreshDiagnostics(), refreshLogs()]);
  } catch (error) {
    showToast(error.message);
  }

  window.setInterval(() => refreshStatus().catch(() => {}), 2500);
  window.setInterval(() => refreshDiagnostics().catch(() => {}), 5000);
  window.setInterval(() => refreshLogs().catch(() => {}), 1200);
}

boot();
