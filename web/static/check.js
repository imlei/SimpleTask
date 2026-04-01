function qs(name) {
  return new URLSearchParams(window.location.search).get(name);
}

(function applyChequeEmbed() {
  if (qs("embed") !== "1") return;
  document.body.classList.add("check-embed");
  document.querySelectorAll('a[href="/settings.html"]').forEach((a) => {
    a.target = "_top";
  });
})();

/** E13B 字段间分隔符（显示用；具体字形以 MICR 字体为准） */
const MICR_DELIM = "\u2446";

let appSettings = {};
let defaultBank = null;

const small = [
  "zero",
  "one",
  "two",
  "three",
  "four",
  "five",
  "six",
  "seven",
  "eight",
  "nine",
  "ten",
  "eleven",
  "twelve",
  "thirteen",
  "fourteen",
  "fifteen",
  "sixteen",
  "seventeen",
  "eighteen",
  "nineteen",
];
const tens = ["", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"];

function wordsUnder100(n) {
  if (n < 20) return small[n];
  const t = Math.floor(n / 10);
  const o = n % 10;
  return tens[t] + (o ? "-" + small[o] : "");
}

function wordsUnder1000(n) {
  const h = Math.floor(n / 100);
  const rest = n % 100;
  let s = "";
  if (h) s = small[h] + " hundred" + (rest ? " " : "");
  if (rest) s += wordsUnder100(rest);
  return s.trim();
}

function intToWords(n) {
  if (!Number.isFinite(n) || n < 0) return "";
  if (n === 0) return "zero";
  if (n >= 1e12) return "amount too large";
  const bi = Math.floor(n / 1e9);
  const mi = Math.floor((n % 1e9) / 1e6);
  const th = Math.floor((n % 1e6) / 1000);
  const re = n % 1000;
  const parts = [];
  if (bi) parts.push(wordsUnder1000(bi) + " billion");
  if (mi) parts.push(wordsUnder1000(mi) + " million");
  if (th) parts.push(wordsUnder1000(th) + " thousand");
  if (re) parts.push(wordsUnder1000(re));
  return parts.join(" ").replace(/\s+/g, " ").trim();
}

function amountToChequeWords(amount) {
  const centsTotal = Math.round(Number(amount) * 100);
  if (!Number.isFinite(centsTotal) || centsTotal < 0) return "";
  const dollars = Math.floor(centsTotal / 100);
  const cents = centsTotal % 100;
  const w = intToWords(dollars);
  if (!w) return "";
  const line = `${w} and ${String(cents).padStart(2, "0")}/100 dollars`;
  return line.toUpperCase();
}

function todayISO() {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function formatDisplayDate(iso) {
  if (!iso) return "";
  const p = iso.split("-");
  if (p.length !== 3) return iso;
  return `${p[0]}-${p[1]}-${p[2]}`;
}

function fmtAmountBox(v, currency) {
  const n = Number(v || 0);
  const c = (currency || "").trim() || "CAD";
  return `${c} ${n.toLocaleString("en-CA", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function digitsOnly(s) {
  return String(s || "").replace(/\D/g, "");
}

function padLeftDigits(s, n) {
  const d = digitsOnly(s);
  if (d.length >= n) return d.slice(-n);
  return d.padStart(n, "0");
}

/**
 * 加拿大（CPA 常见）：字段顺序 — 8 位 FI（Institution 3 + Transit 5）| Account 左补至 12 位 | Cheque 5 位。
 * 美国（ABA 常见）：9 位 Routing | Account（仅数字，不补位）| Cheque 左补至 6 位。
 * 字段间使用 U+2446 分隔；若与开户行要求不一致，请在 Settings 使用「MICR 完全覆盖」。
 */
function buildMicrLine(settings, chequeNo) {
  if (!settings || typeof settings !== "object") return "";
  const ovr = (settings.micrLineOverride || "").trim();
  if (ovr) return ovr;
  const country = String(settings.micrCountry || "CA").toUpperCase();
  const chW = country === "US" ? 6 : 5;
  const ch = padLeftDigits(chequeNo, chW);
  if (country === "US") {
    const rt = padLeftDigits(settings.bankRoutingAba, 9);
    const ac = digitsOnly(settings.bankAccount);
    if (rt.length !== 9 || !ac) return "";
    return MICR_DELIM + rt + MICR_DELIM + ac + MICR_DELIM + ch + MICR_DELIM;
  }
  const inst = padLeftDigits(settings.bankInstitution, 3);
  const tr = padLeftDigits(settings.bankTransit, 5);
  const block8 = inst + tr;
  const ac = padLeftDigits(settings.bankAccount, 12);
  if (!ac) return "";
  return MICR_DELIM + block8 + MICR_DELIM + ac + MICR_DELIM + ch + MICR_DELIM;
}

function updateMicrFormatBanner() {
  const el = document.getElementById("check-micr-banner");
  if (!el) return;
  const us = ((defaultBank && defaultBank.micrCountry) || "CA").toUpperCase() === "US";
  el.textContent = us
    ? "当前 Settings：美国 ABA — MICR 为 Routing（9 位）+ Account + Cheque（6 位）。"
    : "当前 Settings：加拿大 CPA 常用 — MICR 为 FI 8 位（Institution+Transit）+ Account（12 位左补零）+ Cheque（5 位）。";
}

function syncMicr() {
  const chequeEl = document.getElementById("fld-cheque");
  const chequeVal = chequeEl ? chequeEl.value : "";
  const line = buildMicrLine(defaultBank || {}, chequeVal);
  const out = document.getElementById("out-micr");
  const hint = document.getElementById("micr-hint");
  if (out) out.textContent = line;
  if (hint) hint.hidden = !!line;
}

function syncOutputs() {
  const date = document.getElementById("fld-date").value;
  const payee = document.getElementById("fld-payee").value.trim();
  const amount = parseFloat(document.getElementById("fld-amount").value);
  const currency = document.getElementById("fld-currency").value.trim() || "CAD";
  const memo = document.getElementById("fld-memo").value.trim();
  const chq = document.getElementById("fld-cheque")?.value?.trim() || "";
  const company = (appSettings && appSettings.companyName ? String(appSettings.companyName) : "").trim();

  const setText = (id, text) => {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
  };

  setText("out-date", formatDisplayDate(date));
  setText("out-payee", payee);
  setText("out-memo", memo);
  setText("out-company-main", company);
  setText("out-check-no", chq);
  setText("stub-company-1", company);
  setText("stub-company-2", company);
  setText("stub-chq-1", chq);
  setText("stub-chq-2", chq);
  const stubLines = [payee && `Payee: ${payee}`, memo && `Memo: ${memo}`].filter(Boolean);
  const stubText = stubLines.length ? stubLines.join("\n") : "—";
  setText("stub-memo-1", stubText);
  setText("stub-memo-2", stubText);

  const words = Number.isFinite(amount) ? amountToChequeWords(amount) : "";
  setText("out-words", words);
  setText("out-amount", Number.isFinite(amount) ? fmtAmountBox(amount, currency) : "");
  syncMicr();
}

async function loadSettingsForCheck() {
  const r = await fetch("/api/settings", { credentials: "same-origin" });
  if (r.status === 401) {
    window.location.href = "/login.html";
    return;
  }
  if (r.ok) {
    appSettings = await r.json();
  }
  const br = await fetch("/api/bank-accounts/default", { credentials: "same-origin" });
  if (br.status === 401) {
    window.location.href = "/login.html";
    return;
  }
  if (br.ok) {
    defaultBank = await br.json();
    const el = document.getElementById("fld-cheque");
    if (el && !el.dataset.userEdited) {
      el.value = defaultBank.bankChequeNumber || "000001";
    }
  } else {
    defaultBank = null;
  }
  updateMicrFormatBanner();
  syncOutputs();
}

async function loadFromInvoice() {
  const id = qs("id");
  if (!id) {
    document.getElementById("fld-date").value = todayISO();
    const dc = ((defaultBank && defaultBank.defaultChequeCurrency) || appSettings.defaultChequeCurrency || "CAD").trim().toUpperCase();
    document.getElementById("fld-currency").value = dc || "CAD";
    syncOutputs();
    return;
  }
  const r = await fetch(`/api/invoices/${encodeURIComponent(id)}`, { credentials: "same-origin" });
  if (r.status === 401) {
    window.location.href = "/login.html";
    return;
  }
  if (!r.ok) {
    alert("加载发票失败");
    document.getElementById("fld-date").value = todayISO();
    syncOutputs();
    return;
  }
  const inv = await r.json();
  const c = (inv.currency || "CAD").trim();
  const qAmt = qs("amount");
  let bal = Number(inv.balanceDue);
  if (qAmt !== null && qAmt !== "") {
    const parsed = parseFloat(qAmt);
    if (Number.isFinite(parsed) && parsed >= 0) bal = parsed;
  }
  document.getElementById("fld-date").value = todayISO();
  document.getElementById("fld-payee").value = (inv.billToName || "").trim();
  document.getElementById("fld-amount").value = Number.isFinite(bal) ? String(bal) : "0";
  document.getElementById("fld-currency").value = c;
  document.getElementById("fld-memo").value = inv.invoiceNo ? `Invoice ${inv.invoiceNo}` : "";
  syncOutputs();
}

async function saveChequeNextToSettings() {
  const put = await fetch("/api/bank-accounts/default/cheque-next", {
    method: "POST",
    credentials: "same-origin",
  });
  if (put.status === 401) {
    window.location.href = "/login.html";
    return;
  }
  if (!put.ok) {
    alert("保存失败：请先在 Settings 添加并设置默认银行账户");
    return;
  }
  const out = await put.json();
  const next = out.bankChequeNumber || "";
  if (defaultBank) defaultBank = { ...defaultBank, bankChequeNumber: next };
  document.getElementById("fld-cheque").value = next;
  document.getElementById("fld-cheque").dataset.userEdited = "";
  syncOutputs();
}

["fld-date", "fld-payee", "fld-amount", "fld-currency", "fld-memo", "fld-cheque"].forEach((id) => {
  const el = document.getElementById(id);
  if (!el) return;
  el.addEventListener("input", () => {
    if (id === "fld-cheque") el.dataset.userEdited = "1";
    syncOutputs();
  });
  el.addEventListener("change", syncOutputs);
});

document.getElementById("btn-print")?.addEventListener("click", () => window.print());
document.getElementById("btn-back")?.addEventListener("click", () => (window.location.href = "/"));
document.getElementById("btn-cheque-next")?.addEventListener("click", () => saveChequeNextToSettings());

(async function init() {
  await loadSettingsForCheck();
  await loadFromInvoice();
})();
