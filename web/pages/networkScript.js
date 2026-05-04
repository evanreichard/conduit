const state = {
  requests: [],
  selectedID: null,
  activeTab: "preview",
  query: "",
  streamStatus: "connecting",
};

const app = document.getElementById("app");

function init() {
  render();
  connectStream();
}

function connectStream() {
  const es = new EventSource("/stream");

  es.onopen = () => {
    state.streamStatus = "connected";
    render();
  };

  es.onerror = () => {
    state.streamStatus = "disconnected";
    render();
  };

  es.onmessage = (event) => {
    const record = JSON.parse(event.data);
    const foundIdx = state.requests.findIndex((req) => req.ID === record.ID);

    if (foundIdx >= 0) {
      state.requests[foundIdx] = record;
    } else {
      state.requests.unshift(record);
    }

    render();
  };
}

function render() {
  const selected = getSelected();

  app.innerHTML = `
    <section class="flex min-h-dvh flex-col bg-slate-950">
      ${renderHeader()}
      <div class="grid min-h-0 flex-1 grid-cols-1 overflow-hidden lg:grid-cols-[28rem_minmax(0,1fr)]">
        ${renderRequestList()}
        ${renderInspector(selected, false)}
      </div>
      ${selected ? renderMobileSheet(selected) : ""}
    </section>
  `;

  bindEvents();
  renderPreview(selected);
}

function renderHeader() {
  return `
    <header class="border-b border-slate-800 bg-slate-900/80 px-4 py-3 backdrop-blur">
      <div class="flex flex-wrap items-center gap-3">
        <div>
          <h1 class="text-lg font-semibold tracking-tight">Conduit Monitor</h1>
          <p class="text-xs text-slate-400">Live tunnel traffic inspector</p>
        </div>
        <span class="ml-auto rounded-full px-3 py-1 text-xs font-medium ${streamStatusClass()}">${state.streamStatus}</span>
        <button data-action="clear" class="rounded-lg border border-slate-700 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-800">Clear</button>
      </div>
    </header>
  `;
}

function renderRequestList() {
  const filtered = filteredRequests();

  return `
    <aside class="min-h-0 border-r border-slate-800 bg-slate-950 lg:flex lg:flex-col">
      <div class="border-b border-slate-800 p-3">
        <label class="sr-only" for="search">Search requests</label>
        <input id="search" data-input="query" value="${escapeAttr(state.query)}" placeholder="Search path, method, status, type..." class="w-full rounded-xl border border-slate-800 bg-slate-900 px-3 py-2 text-sm outline-none ring-blue-500 placeholder:text-slate-500 focus:ring-2" />
        <div class="mt-2 text-xs text-slate-500">${filtered.length} of ${state.requests.length} requests</div>
      </div>
      <div class="max-h-[calc(100dvh-9rem)] overflow-auto lg:max-h-none lg:flex-1">
        ${filtered.length === 0 ? renderEmptyList() : filtered.map(renderRequestRow).join("")}
      </div>
    </aside>
  `;
}

function renderRequestRow(req) {
  const selected = req.ID === state.selectedID;
  const url = parseURL(req.URL);

  return `
    <button data-select="${req.ID}" class="block w-full border-b border-slate-900 px-4 py-3 text-left transition ${selected ? "bg-blue-950/70" : "hover:bg-slate-900"}">
      <div class="flex items-center gap-2">
        <span class="rounded-md px-2 py-0.5 text-xs font-bold ${methodClass(req.Method)}">${escapeHTML(req.Method || "-")}</span>
        <span class="rounded-md px-2 py-0.5 text-xs font-semibold ${statusClass(req.Status)}">${req.Status || "pending"}</span>
        <span class="ml-auto text-xs text-slate-500">${formatTime(req.Time)}</span>
      </div>
      <div class="mt-2 truncate font-mono text-sm text-slate-100">${escapeHTML(url.path)}</div>
      <div class="mt-1 flex items-center gap-2 truncate text-xs text-slate-500">
        <span class="truncate">${escapeHTML(url.host || req.SourceAddr || "unknown")}</span>
        <span>•</span>
        <span class="truncate">${escapeHTML(req.ResponseBodyType || req.RequestBodyType || "no content type")}</span>
      </div>
    </button>
  `;
}

function renderInspector(selected, mobile) {
  if (!selected) {
    return `
      <section class="hidden min-h-0 items-center justify-center bg-slate-950 p-8 lg:flex">
        <div class="max-w-md text-center">
          <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-slate-900 text-2xl">↯</div>
          <h2 class="text-lg font-semibold">No request selected</h2>
          <p class="mt-2 text-sm text-slate-400">Send traffic through a tunnel, then select a request to inspect headers, bodies, and previews.</p>
        </div>
      </section>
    `;
  }

  const wrapperClass = mobile ? "flex h-full flex-col bg-slate-950" : "hidden min-h-0 flex-col bg-slate-950 lg:flex";
  return `
    <section class="${wrapperClass}">
      ${renderInspectorHeader(selected, mobile)}
      ${renderTabs()}
      <div class="min-h-0 flex-1 overflow-auto p-4">
        ${renderActiveTab(selected)}
      </div>
    </section>
  `;
}

function renderMobileSheet(selected) {
  return `
    <div class="fixed inset-0 z-50 bg-slate-950 lg:hidden">
      ${renderInspector(selected, true)}
    </div>
  `;
}

function renderInspectorHeader(req, mobile) {
  const url = parseURL(req.URL);

  return `
    <div class="border-b border-slate-800 p-4">
      <div class="flex items-start gap-3">
        ${mobile ? `<button data-action="close" class="rounded-lg border border-slate-700 px-3 py-1.5 text-sm">Back</button>` : ""}
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="rounded-md px-2 py-0.5 text-xs font-bold ${methodClass(req.Method)}">${escapeHTML(req.Method || "-")}</span>
            <span class="rounded-md px-2 py-0.5 text-xs font-semibold ${statusClass(req.Status)}">${req.Status || "pending"}</span>
            <span class="text-xs text-slate-500">${formatTime(req.Time)}</span>
          </div>
          <h2 class="mt-2 truncate font-mono text-base font-semibold">${escapeHTML(url.path)}</h2>
          <p class="mt-1 truncate text-xs text-slate-500">${escapeHTML(req.URL || "")}</p>
        </div>
        <button data-copy="${escapeAttr(req.URL || "")}" class="rounded-lg border border-slate-700 px-3 py-1.5 text-sm hover:bg-slate-800">Copy URL</button>
      </div>
    </div>
  `;
}

function renderTabs() {
  return `
    <nav class="flex gap-1 overflow-x-auto border-b border-slate-800 px-3 py-2">
      ${["preview", "overview", "request", "response"].map((tab) => `
        <button data-tab="${tab}" class="rounded-lg px-3 py-1.5 text-sm font-medium capitalize ${state.activeTab === tab ? "bg-blue-600 text-white" : "text-slate-400 hover:bg-slate-900 hover:text-slate-100"}">${tab}</button>
      `).join("")}
    </nav>
  `;
}

function renderActiveTab(req) {
  if (state.activeTab === "overview") return renderOverview(req);
  if (state.activeTab === "request") return renderMessage(req, "Request");
  if (state.activeTab === "response") return renderMessage(req, "Response");
  return `<div data-preview-root></div>`;
}

function renderOverview(req) {
  return `
    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      ${summaryCard("Method", req.Method || "-")}
      ${summaryCard("Status", req.Status || "pending")}
      ${summaryCard("Source", req.SourceAddr || "-")}
      ${summaryCard("Request Body", bodySummary(req, "Request"))}
      ${summaryCard("Response Body", bodySummary(req, "Response"))}
      ${summaryCard("Content Type", req.ResponseBodyType || req.RequestBodyType || "-")}
    </div>
  `;
}

function renderMessage(req, prefix) {
  const headers = req[`${prefix}Headers`] || {};
  const body = decodeBody(req[`${prefix}Body`]);

  return `
    <div class="space-y-4">
      <section>
        <div class="mb-2 flex items-center justify-between gap-2">
          <h3 class="font-semibold">Headers</h3>
          <button data-copy="${escapeAttr(formatHeaders(headers))}" class="rounded-lg border border-slate-700 px-3 py-1 text-xs hover:bg-slate-800">Copy</button>
        </div>
        <pre class="overflow-auto rounded-xl border border-slate-800 bg-slate-900 p-3 text-xs text-slate-300">${escapeHTML(formatHeaders(headers) || "No headers")}</pre>
      </section>
      <section>
        <div class="mb-2 flex items-center justify-between gap-2">
          <h3 class="font-semibold">Body</h3>
          <button data-copy="${escapeAttr(body)}" class="rounded-lg border border-slate-700 px-3 py-1 text-xs hover:bg-slate-800">Copy</button>
        </div>
        <p class="mb-2 text-xs text-slate-500">${escapeHTML(bodySummary(req, prefix))}</p>
        <pre class="max-h-[32rem] overflow-auto rounded-xl border border-slate-800 bg-slate-900 p-3 text-xs text-slate-300">${escapeHTML(formatBody(body, req[`${prefix}BodyType`]))}</pre>
      </section>
    </div>
  `;
}

function renderPreview(selected) {
  const roots = document.querySelectorAll("[data-preview-root]");
  if (roots.length === 0 || !selected) return;

  roots.forEach((root) => renderPreviewInto(root, selected));
}

function renderPreviewInto(root, selected) {
  const contentType = selected.ResponseBodyType || "";
  const body = selected.ResponseBody;

  if (!selected.ResponseBodyCaptured || !body) {
    root.innerHTML = renderPreviewEmpty(selected);
    return;
  }

  if (isImage(contentType)) {
    root.innerHTML = `<div class="rounded-xl border border-slate-800 bg-slate-900 p-4"><img class="mx-auto max-h-[70dvh] max-w-full rounded-lg" alt="Response preview" src="data:${escapeAttr(contentType)};base64,${body}" /></div>`;
    return;
  }

  const decoded = decodeBody(body);
  if (isHTML(contentType)) {
    root.innerHTML = `<iframe title="Response HTML preview" sandbox class="h-[70dvh] w-full rounded-xl border border-slate-800 bg-white"></iframe>`;
    root.querySelector("iframe").srcdoc = decoded;
    return;
  }

  root.innerHTML = `<pre class="max-h-[70dvh] overflow-auto rounded-xl border border-slate-800 bg-slate-900 p-4 text-sm text-slate-300">${escapeHTML(formatBody(decoded, contentType))}</pre>`;
}

function renderPreviewEmpty(req) {
  const reason = req.ResponseBodySkipped || "No captured response body is available.";
  return `
    <div class="rounded-xl border border-dashed border-slate-700 p-8 text-center">
      <h3 class="font-semibold">Nothing to preview</h3>
      <p class="mt-2 text-sm text-slate-400">${escapeHTML(reason)}</p>
      <p class="mt-1 text-xs text-slate-500">${escapeHTML(bodySummary(req, "Response"))}</p>
    </div>
  `;
}

function bindEvents() {
  document.querySelectorAll("[data-select]").forEach((el) => {
    el.addEventListener("click", () => {
      state.selectedID = el.dataset.select;
      state.activeTab = "preview";
      render();
    });
  });

  document.querySelectorAll("[data-tab]").forEach((el) => {
    el.addEventListener("click", () => {
      state.activeTab = el.dataset.tab;
      render();
    });
  });

  document.querySelectorAll("[data-action='clear']").forEach((el) => {
    el.addEventListener("click", () => {
      state.requests = [];
      state.selectedID = null;
      render();
    });
  });

  document.querySelectorAll("[data-action='close']").forEach((el) => {
    el.addEventListener("click", () => {
      state.selectedID = null;
      render();
    });
  });

  document.querySelectorAll("[data-copy]").forEach((el) => {
    el.addEventListener("click", () => navigator.clipboard?.writeText(el.dataset.copy || ""));
  });

  const search = document.querySelector("[data-input='query']");
  if (search) {
    search.addEventListener("input", (event) => {
      state.query = event.target.value;
      render();
      document.querySelector("[data-input='query']")?.focus();
    });
  }
}

function filteredRequests() {
  const query = state.query.trim().toLowerCase();
  if (!query) return state.requests;

  return state.requests.filter((req) => [
    req.URL,
    req.Method,
    String(req.Status || "pending"),
    req.SourceAddr,
    req.RequestBodyType,
    req.ResponseBodyType,
  ].some((value) => String(value || "").toLowerCase().includes(query)));
}

function getSelected() {
  return state.requests.find((req) => req.ID === state.selectedID) || null;
}

function parseURL(raw) {
  try {
    const parsed = new URL(raw, window.location.origin);
    return { host: parsed.host, path: `${parsed.pathname}${parsed.search}` || raw };
  } catch {
    return { host: "", path: raw || "-" };
  }
}

function decodeBody(base64Data) {
  if (!base64Data) return "";
  const binary = atob(base64Data);
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

function formatBody(body, contentType = "") {
  if (isJSON(contentType)) {
    try {
      return JSON.stringify(JSON.parse(body), null, 2);
    } catch {
      return body;
    }
  }
  return body || "No body";
}

function formatHeaders(headers) {
  return Object.entries(headers || {})
    .map(([key, values]) => `${key}: ${Array.isArray(values) ? values.join(", ") : values}`)
    .join("\n");
}

function bodySummary(req, prefix) {
  const size = req[`${prefix}BodySize`];
  const captured = req[`${prefix}BodyCaptured`];
  const truncated = req[`${prefix}BodyTruncated`];
  const skipped = req[`${prefix}BodySkipped`];

  if (captured) return `${formatBytes(size)} captured${truncated ? " (truncated)" : ""}`;
  if (skipped) return skipped;
  if (size > 0) return `${formatBytes(size)} not captured`;
  return "No body";
}

function summaryCard(label, value) {
  return `<div class="rounded-xl border border-slate-800 bg-slate-900 p-4"><div class="text-xs uppercase tracking-wide text-slate-500">${escapeHTML(label)}</div><div class="mt-1 break-words text-sm font-medium text-slate-100">${escapeHTML(String(value))}</div></div>`;
}

function renderEmptyList() {
  return `<div class="p-8 text-center text-sm text-slate-500">No requests yet. Start sending traffic through your tunnel.</div>`;
}

function isJSON(contentType) {
  return /(^|\/)json($|;)|\+json($|;)/i.test(contentType || "");
}

function isHTML(contentType) {
  return /text\/html/i.test(contentType || "");
}

function isImage(contentType) {
  return /^image\/(png|jpe?g|gif|webp|svg\+xml)/i.test(contentType || "");
}

function methodClass(method) {
  return {
    GET: "bg-emerald-500/15 text-emerald-300",
    POST: "bg-blue-500/15 text-blue-300",
    PUT: "bg-amber-500/15 text-amber-300",
    PATCH: "bg-purple-500/15 text-purple-300",
    DELETE: "bg-red-500/15 text-red-300",
  }[method] || "bg-slate-700 text-slate-200";
}

function statusClass(status) {
  if (!status) return "bg-slate-700 text-slate-300";
  if (status < 300) return "bg-emerald-500/15 text-emerald-300";
  if (status < 400) return "bg-blue-500/15 text-blue-300";
  if (status < 500) return "bg-amber-500/15 text-amber-300";
  return "bg-red-500/15 text-red-300";
}

function streamStatusClass() {
  if (state.streamStatus === "connected") return "bg-emerald-500/15 text-emerald-300";
  if (state.streamStatus === "connecting") return "bg-amber-500/15 text-amber-300";
  return "bg-red-500/15 text-red-300";
}

function formatTime(time) {
  if (!time) return "-";
  return new Date(time).toLocaleTimeString();
}

function formatBytes(bytes) {
  if (!bytes || bytes < 0) return "unknown size";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>'"]/g, (char) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "'": "&#39;",
    '"': "&quot;",
  })[char]);
}

function escapeAttr(value) {
  return escapeHTML(value).replace(/`/g, "&#96;");
}

init();
