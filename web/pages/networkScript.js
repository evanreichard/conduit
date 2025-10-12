function networkMonitor() {
  return {
    requests: [],
    selected: null,

    init() {
      const es = new EventSource("/stream");
      es.onmessage = (e) => {
        const record = JSON.parse(e.data);
        const foundIdx = this.requests.findIndex((r) => r.ID === record.ID);
        if (foundIdx >= 0) {
          this.requests[foundIdx] = record;
        } else {
          this.requests.unshift(record);
        }
      };
    },

    clear() {
      this.requests = [];
      this.selected = null;
    },

    statusColor(status) {
      if (!status) return "text-gray-400";
      if (status < 300) return "text-green-400";
      if (status < 400) return "text-blue-400";
      if (status < 500) return "text-yellow-400";
      return "text-red-400";
    },

    formatTime(time) {
      return new Date(time).toLocaleTimeString();
    },
  };
}

function formatData(base64Data) {
  if (!base64Data) return "";
  try {
    const decoded = atob(base64Data);
    const parsed = JSON.parse(decoded);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return atob(base64Data);
  }
}
