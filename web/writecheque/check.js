"use strict";

// ── Helpers ──────────────────────────────────────────────────────────────────

function qs(name) {
  return new URLSearchParams(window.location.search).get(name);
}

function escHtml(s) {
  return String(s || "")
    .replace(/&/g, "&amp;").replace(/</g, "&lt;")
    .replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

function todayISO() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,"0")}-${String(d.getDate()).padStart(2,"0")}`;
}

function get(id) { return document.getElementById(id); }
function val(id) { return (get(id)?.value || "").trim(); }

// ── State ─────────────────────────────────────────────────────────────────────

let allBanks = [];
let defaultBankId = "";
let companiesList = [];

// ── Preview (iframe) ──────────────────────────────────────────────────────────

function previewURL() {
  const p = new URLSearchParams();
  const bankId = val("fld-bank");
  if (bankId) p.set("bank_id", bankId);
  p.set("payee",    val("fld-payee"));
  p.set("amount",   val("fld-amount") || "0");
  p.set("currency", val("fld-currency"));
  p.set("memo",     val("fld-memo"));
  p.set("date",     val("fld-date"));
  p.set("check_no", val("fld-cheque"));
  return "/api/writecheque/preview?" + p.toString();
}

function refreshPreview() {
  const iframe = get("cheque-preview");
  if (!iframe) return;
  iframe.src = previewURL();
}

// ── Bank account selector ─────────────────────────────────────────────────────

function populateBankSelect() {
  const sel = get("fld-bank");
  if (!sel) return;
  sel.innerHTML = allBanks.map(b =>
    `<option value="${escHtml(b.id)}"${b.id === defaultBankId ? " selected" : ""}>${escHtml(b.label || b.id)}</option>`
  ).join("");
}

function onBankChange() {
  const id = val("fld-bank");
  const b = allBanks.find(x => x.id === id);
  if (!b) return;
  defaultBankId = id;
  // Update cheque number and currency from selected bank
  const cheqEl = get("fld-cheque");
  if (cheqEl && !cheqEl.dataset.userEdited) cheqEl.value = b.bankChequeNumber || "000001";
  const curEl = get("fld-currency");
  if (curEl) curEl.value = (b.defaultChequeCurrency || "CAD").toUpperCase();
  updateMicrBanner(b);
  refreshPreview();
}

function updateMicrBanner(bank) {
  const el = get("check-micr-banner");
  if (!el || !bank) return;
  const country = (bank.micrCountry || "CA").toUpperCase();
  if (country === "US") {
    el.textContent = "当前账户：美国 ABA — MICR 为 Routing（9 位）+ Account + Cheque（6 位）。";
  } else if (country === "EU") {
    el.textContent = "当前账户：欧洲账户（EU）— 默认不生成 MICR，可使用 MICR Override。";
  } else {
    el.textContent = "当前账户：加拿大 CPA — MICR 为 FI 8 位（Institution+Transit）+ Account（12 位）+ Cheque（5 位）。";
  }
}

// ── Load from URL params (Payroll integration) ────────────────────────────────

function applyURLParams() {
  const payee = qs("payee");
  const amount = qs("amount");
  const memo = qs("memo");
  const date = qs("date");
  const currency = qs("currency");
  const bankId = qs("bank_id");
  const checkNo = qs("check_no");

  if (payee !== null)  { const el = get("fld-payee");    if (el) el.value = payee; }
  if (amount !== null) { const el = get("fld-amount");   if (el) el.value = amount; }
  if (memo !== null)   { const el = get("fld-memo");     if (el) el.value = memo; }
  if (date !== null)   { const el = get("fld-date");     if (el) el.value = date; }
  if (currency !== null) {
    const el = get("fld-currency");
    if (el) el.value = currency.toUpperCase();
  }
  if (bankId !== null) {
    const el = get("fld-bank");
    if (el) el.value = bankId;
  }
  if (checkNo !== null) {
    const el = get("fld-cheque");
    if (el) { el.value = checkNo; el.dataset.userEdited = "1"; }
  }
}

// ── API calls ─────────────────────────────────────────────────────────────────

async function fetchBankAccounts() {
  const r = await fetch("/api/bank-accounts", { credentials: "same-origin" });
  if (r.status === 401) { window.location.href = "/login.html"; return; }
  if (!r.ok) return;
  const data = await r.json();
  allBanks = Array.isArray(data.items) ? data.items : [];
  defaultBankId = data.defaultId || "";
}

async function fetchCompanies() {
  const r = await fetch("/api/payroll/companies?status=active", { credentials: "same-origin" });
  if (!r.ok) return;
  const data = await r.json();
  companiesList = Array.isArray(data) ? data : (Array.isArray(data.items) ? data.items : []);
}

async function saveChequeNext() {
  const r = await fetch("/api/bank-accounts/default/cheque-next", {
    method: "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json; charset=utf-8" },
    body: "{}",
  });
  if (r.ok) {
    const d = await r.json();
    const el = get("fld-cheque");
    if (el && d.bankChequeNumber) el.value = d.bankChequeNumber;
    refreshPreview();
    alert("支票号已更新为 " + (d.bankChequeNumber || ""));
  }
}

// ── Bank list rendering ───────────────────────────────────────────────────────

function renderBankList() {
  const box = get("check-bank-list");
  if (!box) return;
  if (!allBanks.length) {
    box.innerHTML = `<div style="color:var(--muted);font-size:13px">No bank accounts yet. Please add one.</div>`;
    return;
  }
  box.innerHTML = allBanks.map(b => {
    const isDef = b.id === defaultBankId;
    const bankDisplay = b.bankName || (b.micrCountry === "US"
      ? `Routing ${b.bankRoutingAba || "-"}`
      : b.micrCountry === "EU" ? `IBAN ${b.bankIban || "-"}`
      : `FI ${b.bankInstitution || ""}-${b.bankTransit || ""}`);
    return `<div class="bank-item-row">
      <div><div class="bank-col-title">Account Name</div><div class="bank-col-value"><strong>${escHtml(b.label || b.id)}</strong>${isDef ? " (Default)" : ""}</div></div>
      <div><div class="bank-col-title">Bank</div><div class="bank-col-value">${escHtml(bankDisplay)}</div></div>
      <div><div class="bank-col-title">Currency</div><div class="bank-col-value">${escHtml((b.defaultChequeCurrency||"CAD").toUpperCase())}</div></div>
      <div><div class="bank-col-title">Account</div><div class="bank-col-value">${escHtml(b.bankAccount||"")}</div></div>
      <div class="bank-row-actions">
        <button type="button" data-edit-id="${escHtml(b.id)}">Edit</button>
        <button type="button" data-write-id="${escHtml(b.id)}">Write Cheque</button>
      </div>
    </div>`;
  }).join("");

  box.querySelectorAll("[data-edit-id]").forEach(btn =>
    btn.addEventListener("click", () => openEditBank(btn.getAttribute("data-edit-id")))
  );
  box.querySelectorAll("[data-write-id]").forEach(btn =>
    btn.addEventListener("click", async () => {
      const id = btn.getAttribute("data-write-id");
      await setDefaultBank(id);
      activateMenu("write");
    })
  );
}

async function setDefaultBank(id) {
  await fetch(`/api/bank-accounts/${encodeURIComponent(id)}/default`, {
    method: "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json; charset=utf-8" },
    body: "{}",
  });
  defaultBankId = id;
  const b = allBanks.find(x => x.id === id);
  if (b) {
    const cheqEl = get("fld-cheque");
    if (cheqEl) { cheqEl.value = b.bankChequeNumber || "000001"; cheqEl.dataset.userEdited = ""; }
    const curEl = get("fld-currency");
    if (curEl) curEl.value = (b.defaultChequeCurrency || "CAD").toUpperCase();
    updateMicrBanner(b);
  }
  populateBankSelect();
  renderBankList();
  refreshPreview();
}

// ── Bank form ─────────────────────────────────────────────────────────────────

function syncCountryFields() {
  const country = (get("new-bank-country")?.value || "CA").toUpperCase();
  document.querySelectorAll(".bank-field-ca").forEach(el => { el.hidden = country !== "CA"; });
  document.querySelectorAll(".bank-field-us").forEach(el => { el.hidden = country !== "US"; });
  document.querySelectorAll(".bank-field-eu").forEach(el => { el.hidden = country !== "EU"; });
}

function populateCompanySelect() {
  const sel = get("new-bank-company");
  if (!sel) return;
  const existing = sel.innerHTML.split("\n")[0]; // keep first "— Not linked —" option
  sel.innerHTML = `<option value="">— Not linked —</option>` +
    companiesList.map(c => `<option value="${escHtml(c.id)}">${escHtml(c.name || c.legalName || c.id)}</option>`).join("");
}

function clearBankForm() {
  ["new-bank-id","new-bank-label","new-bank-name","new-bank-address",
   "new-bank-city","new-bank-postal","new-bank-institution","new-bank-transit",
   "new-bank-routing","new-bank-iban","new-bank-swift","new-bank-account","new-bank-micr"
  ].forEach(id => { const el = get(id); if (el) el.value = ""; });
  const title = get("bank-form-title");
  if (title) title.textContent = "Add New Acct";
  const sel = id => { const el = get(id); if (el) el.value = ""; };
  sel("new-bank-company");
  if (get("new-bank-country")) get("new-bank-country").value = "CA";
  if (get("new-bank-currency")) get("new-bank-currency").value = "CAD";
  if (get("new-bank-province")) get("new-bank-province").value = "";
  if (get("new-bank-cheque")) get("new-bank-cheque").value = "000001";
  if (get("btn-bank-delete")) get("btn-bank-delete").hidden = true;
  syncCountryFields();
}

function openEditBank(id) {
  const b = allBanks.find(x => x.id === id);
  if (!b) return;
  const set = (elId, v) => { const el = get(elId); if (el) el.value = v || ""; };
  set("new-bank-id", b.id);
  set("new-bank-label", b.label);
  set("new-bank-company", b.companyId);
  set("new-bank-name", b.bankName);
  set("new-bank-address", b.bankAddress);
  set("new-bank-city", b.bankCity);
  set("new-bank-province", b.bankProvince);
  set("new-bank-postal", b.bankPostalCode);
  set("new-bank-country", (b.micrCountry || "CA").toUpperCase());
  set("new-bank-institution", b.bankInstitution);
  set("new-bank-transit", b.bankTransit);
  set("new-bank-routing", b.bankRoutingAba);
  set("new-bank-iban", b.bankIban);
  set("new-bank-swift", b.bankSwift);
  set("new-bank-account", b.bankAccount);
  set("new-bank-cheque", b.bankChequeNumber || "000001");
  set("new-bank-currency", (b.defaultChequeCurrency || "CAD").toUpperCase());
  set("new-bank-micr", b.micrLineOverride);
  const title = get("bank-form-title");
  if (title) title.textContent = "Edit Bank Account";
  if (get("btn-bank-delete")) get("btn-bank-delete").hidden = false;
  syncCountryFields();
  activateMenu("add");
}

async function saveBankAccount() {
  const editID = val("new-bank-id");
  const body = {
    label:                 val("new-bank-label"),
    companyId:             val("new-bank-company"),
    bankName:              val("new-bank-name"),
    bankAddress:           val("new-bank-address"),
    bankCity:              val("new-bank-city"),
    bankProvince:          val("new-bank-province"),
    bankPostalCode:        val("new-bank-postal"),
    micrCountry:           val("new-bank-country") || "CA",
    bankInstitution:       val("new-bank-institution"),
    bankTransit:           val("new-bank-transit"),
    bankRoutingAba:        val("new-bank-routing"),
    bankIban:              val("new-bank-iban"),
    bankSwift:             val("new-bank-swift"),
    bankAccount:           val("new-bank-account"),
    bankChequeNumber:      val("new-bank-cheque"),
    defaultChequeCurrency: val("new-bank-currency") || "CAD",
    micrLineOverride:      val("new-bank-micr"),
  };
  const url = editID ? `/api/bank-accounts/${encodeURIComponent(editID)}` : "/api/bank-accounts";
  const r = await fetch(url, {
    method: editID ? "PUT" : "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json; charset=utf-8" },
    body: JSON.stringify(body),
  });
  if (r.status === 401) { window.location.href = "/login.html"; return; }
  if (!r.ok) { alert("保存失败: " + (await r.text())); return; }
  clearBankForm();
  await fetchBankAccounts();
  populateBankSelect();
  renderBankList();
  activateMenu("list");
}

async function deleteBankAccount() {
  const id = val("new-bank-id");
  if (!id || !confirm("Delete this bank account?")) return;
  const r = await fetch(`/api/bank-accounts/${encodeURIComponent(id)}`, {
    method: "DELETE", credentials: "same-origin",
  });
  if (!r.ok) { alert("删除失败"); return; }
  clearBankForm();
  await fetchBankAccounts();
  populateBankSelect();
  renderBankList();
  activateMenu("list");
}

// ── Menu / panel switching ────────────────────────────────────────────────────

function activateMenu(tab) {
  const map = {
    write: { btn: "menu-write-cheque",  panel: "panel-write-cheque-main" },
    list:  { btn: "menu-bank-list",     panel: "panel-bank-list-main" },
    add:   { btn: "menu-bank-add",      panel: "panel-bank-add-main" },
  };
  Object.entries(map).forEach(([t, ids]) => {
    get(ids.btn)?.classList.toggle("active", t === tab);
    const panel = get(ids.panel);
    if (panel) panel.hidden = t !== tab;
  });
  if (tab === "write") refreshPreview();
}

// ── Back button ───────────────────────────────────────────────────────────────

function initBackButton() {
  const btn = get("btn-back");
  if (!btn) return;
  const ref = document.referrer;
  if (ref && ref !== window.location.href) {
    btn.addEventListener("click", () => history.back());
  } else {
    btn.addEventListener("click", () => window.location.href = "/");
  }
}

// ── Init ──────────────────────────────────────────────────────────────────────

async function init() {
  await Promise.all([fetchBankAccounts(), fetchCompanies()]);

  populateBankSelect();
  populateCompanySelect();
  renderBankList();

  // Set form defaults
  const dateEl = get("fld-date");
  if (dateEl && !dateEl.value) dateEl.value = todayISO();

  const defBank = allBanks.find(b => b.id === defaultBankId) || allBanks[0];
  if (defBank) {
    if (get("fld-cheque")) get("fld-cheque").value = defBank.bankChequeNumber || "000001";
    if (get("fld-currency")) get("fld-currency").value = (defBank.defaultChequeCurrency || "CAD").toUpperCase();
    if (get("fld-bank")) get("fld-bank").value = defBank.id;
    updateMicrBanner(defBank);
  }

  // Apply URL params (Payroll integration)
  applyURLParams();

  // If URL params include payee/amount, go straight to write panel
  if (qs("payee") !== null || qs("amount") !== null) {
    activateMenu("write");
  }

  // Auto-open print dialog if ?print=1
  if (qs("print") === "1") {
    activateMenu("write");
    setTimeout(() => {
      const iframe = get("cheque-preview");
      if (iframe && iframe.contentWindow) {
        iframe.onload = () => iframe.contentWindow.print();
      }
    }, 200);
  }

  // Bind form inputs → refresh preview
  ["fld-date","fld-payee","fld-amount","fld-memo","fld-cheque"].forEach(id => {
    const el = get(id);
    if (!el) return;
    el.addEventListener("input", refreshPreview);
    if (id === "fld-cheque") el.addEventListener("input", () => { el.dataset.userEdited = "1"; });
  });
  get("fld-currency")?.addEventListener("change", refreshPreview);
  get("fld-bank")?.addEventListener("change", onBankChange);

  // Sidebar menu
  get("menu-write-cheque")?.addEventListener("click", () => activateMenu("write"));
  get("menu-bank-list")?.addEventListener("click", () => activateMenu("list"));
  get("menu-bank-add")?.addEventListener("click", () => { clearBankForm(); activateMenu("add"); });

  // Toolbar buttons
  get("btn-print")?.addEventListener("click", () => {
    const iframe = get("cheque-preview");
    if (iframe?.contentWindow) iframe.contentWindow.print();
  });
  get("btn-cheque-next")?.addEventListener("click", saveChequeNext);

  // Bank form buttons
  get("btn-add-bank")?.addEventListener("click", saveBankAccount);
  get("btn-bank-cancel")?.addEventListener("click", () => { clearBankForm(); activateMenu("list"); });
  get("btn-bank-delete")?.addEventListener("click", deleteBankAccount);
  get("new-bank-country")?.addEventListener("change", syncCountryFields);

  initBackButton();
  syncCountryFields();
}

document.addEventListener("DOMContentLoaded", init);
