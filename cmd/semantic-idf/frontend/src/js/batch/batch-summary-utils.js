const SUMMARY_NUMBER_PATTERN = /^[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][-+]?\d+)?/;

export function parseSummaryNumber(value) {
  const text = String(value ?? "").trim();
  const token = text.match(SUMMARY_NUMBER_PATTERN)?.[0] || "";
  if (!token) {
    return { ok: false, value: 0, token: "" };
  }
  const number = Number(token);
  return Number.isFinite(number) ? { ok: true, value: number, token } : { ok: false, value: 0, token: "" };
}

export function summaryUnit(metric, displayValue) {
  const unit = String(metric?.unit || "").trim();
  if (unit) {
    return unit;
  }
  const text = String(displayValue ?? "").trim();
  const number = parseSummaryNumber(text);
  return number.ok ? text.slice(number.token.length).trim() : "";
}
