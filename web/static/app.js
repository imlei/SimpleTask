const $ = (sel, el = document) => el.querySelector(sel);

/** 本机当日日期 YYYY-MM-DD（与 date 输入框一致） */
function todayLocalISO() {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

/** 当前年月 YYYY-MM（用于 month 输入框） */
function currentMonthISO() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

async function api(path, opts = {}) {
  const r = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json; charset=utf-8", ...opts.headers },
    ...opts,
  });
  if (r.status === 401) {
    window.location.href = "/login.html";
    throw new Error("unauthorized");
  }
  if (r.status === 204) return null;
  const text = await r.text();
  if (!r.ok) throw new Error(text || r.statusText);
  return text ? JSON.parse(text) : null;
}

/** 列表接口：后端空切片曾可能序列化为 JSON null，解析后需当数组使用 */
function asArray(v) {
  return Array.isArray(v) ? v : [];
}

// --- Tabs ---
document.querySelectorAll(".tab").forEach((btn) => {
  btn.addEventListener("click", () => {
    const tab = btn.dataset.tab;
    document.querySelectorAll(".tab").forEach((b) => b.classList.toggle("active", b === btn));
    document.querySelectorAll(".panel").forEach((p) => p.classList.toggle("active", p.id === `panel-${tab}`));
    if (tab === "tasks") loadTasks();
    if (tab === "prices") loadPrices();
    if (tab === "report") {
      const rm = document.getElementById("report-month");
      if (rm && !rm.value) rm.value = currentMonthISO();
      loadReport();
    }
    if (tab === "invoices") loadInvoices();
  });
});

// --- Tasks ---
let tasksCache = [];
/** 当前编辑对话框打开时的任务快照（用于保留 completedAt 等未在表单展示的字段） */
let lastOpenedTask = null;

function taskToPayload(t) {
  return {
    companyName: t.companyName || "",
    date: t.date || "",
    service1: t.service1 || "",
    service2: "",
    price1: Number(t.price1) || 0,
    price2: 0,
    selectedPriceIds: Array.isArray(t.selectedPriceIds) ? t.selectedPriceIds : [],
    status: t.status || "Pending",
    completedAt: t.completedAt || "",
    note: t.note || "",
  };
}

async function loadTasks() {
  try {
    tasksCache = asArray(await api("/api/tasks"));
    renderTasks();
  } catch (e) {
    console.error(e);
    alert("加载任务失败: " + e.message);
  }
}

function renderTasks() {
  const filter = $("#filter-status").value;
  const body = $("#tasks-body");
  body.innerHTML = "";
  const rows = asArray(tasksCache).filter((t) => !filter || t.status === filter);
  for (const t of rows) {
    const tr = document.createElement("tr");
    const done = t.status === "Done";
    tr.innerHTML = `
      <td>${escapeHtml(t.id)}</td>
      <td>${escapeHtml(t.companyName)}</td>
      <td>${escapeHtml(t.date || "")}</td>
      <td>${escapeHtml(t.service1 || "")}</td>
      <td>${fmtNum(t.price1)}</td>
      <td><span class="${done ? "status-done" : "status-pending"}">${escapeHtml(t.status)}</span></td>
      <td>${escapeHtml(t.completedAt || "")}</td>
      <td>${escapeHtml(t.note || "")}</td>
      <td class="row-actions">
        <button type="button" class="ghost" data-act="edit">编辑</button>
        <button type="button" class="ghost" data-act="invoice">Invoice</button>
        <button type="button" class="ghost success" data-act="done" ${done ? "disabled" : ""}>Completed</button>
        <button type="button" class="ghost danger" data-act="del">删除</button>
      </td>`;
    tr.querySelector('[data-act="edit"]').addEventListener("click", () => openTaskDialog(t));
    tr.querySelector('[data-act="invoice"]').addEventListener("click", () => openInvoiceDialog(t));
    const btnDone = tr.querySelector('[data-act="done"]');
    if (!done) {
      btnDone.addEventListener("click", () => markTaskCompleted(t));
    }
    tr.querySelector('[data-act="del"]').addEventListener("click", () => deleteTask(t.id));
    body.appendChild(tr);
  }
}

$("#filter-status").addEventListener("change", renderTasks);

$("#btn-new-task").addEventListener("click", () => openTaskDialog(null));

const dlgTask = $("#dlg-task");
const formTask = $("#form-task");
const taskPricePicks = $("#task-price-picks");

taskPricePicks.addEventListener("change", () => applyTaskPriceSelection());

$("#task-price-clear").addEventListener("click", () => {
  taskPricePicks.querySelectorAll('input[type="checkbox"]').forEach((cb) => {
    cb.checked = false;
  });
  applyTaskPriceSelection();
});

async function ensurePricesLoaded() {
  if (asArray(pricesCache).length) return;
  try {
    pricesCache = asArray(await api("/api/prices"));
  } catch (e) {
    console.error(e);
    alert("加载价目表失败，无法多选: " + e.message);
  }
}

function renderPriceCheckboxes(selectedIds) {
  const sel = new Set(selectedIds || []);
  taskPricePicks.innerHTML = "";
  const sorted = [...asArray(pricesCache)].sort((a, b) => a.id.localeCompare(b.id));
  for (const p of sorted) {
    const label = document.createElement("label");
    label.className = "price-pick-row";
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.dataset.id = p.id;
    cb.checked = sel.has(p.id);
    const span = document.createElement("span");
    const strong = document.createElement("strong");
    strong.textContent = p.serviceName;
    const meta = document.createElement("span");
    meta.className = "price-pick-meta";
    let metaText =
      p.amount != null && p.amount !== undefined
        ? ` ${p.amount} ${p.currency}`
        : " 未定价";
    if (p.note) metaText += " " + p.note;
    meta.textContent = metaText;
    span.appendChild(strong);
    span.appendChild(meta);
    label.appendChild(cb);
    label.appendChild(span);
    taskPricePicks.appendChild(label);
  }
}

function getSelectedPriceIds() {
  const out = [];
  taskPricePicks.querySelectorAll('input[type="checkbox"]:checked').forEach((cb) => {
    out.push(cb.dataset.id);
  });
  return out;
}

/** 根据当前勾选同步「业务一」「价格一」（勾选变化时调用；多币种时其余写在预览区） */
function applyTaskPriceSelection() {
  const ids = getSelectedPriceIds();
  const preview = $("#task-price-preview");
  if (ids.length === 0) {
    $("#task-s1").value = "";
    $("#task-p1").value = "";
    preview.hidden = true;
    preview.textContent = "";
    return;
  }
  const pc = asArray(pricesCache);
  const items = ids
    .map((id) => pc.find((x) => x.id === id))
    .filter(Boolean);
  $("#task-s1").value = items.map((p) => p.serviceName).join("；");

  const byCur = {};
  const curOrder = [];
  for (const id of ids) {
    const p = pc.find((x) => x.id === id);
    if (!p || p.amount == null || p.amount === undefined) continue;
    const c = p.currency || "CNY";
    if (byCur[c] === undefined) {
      byCur[c] = 0;
      curOrder.push(c);
    }
    byCur[c] += Number(p.amount);
  }
  if (curOrder.length === 0) {
    $("#task-p1").value = "";
    preview.hidden = true;
    preview.textContent = "";
  } else {
    $("#task-p1").value = byCur[curOrder[0]];
    if (curOrder.length > 1) {
      preview.hidden = false;
      preview.textContent =
        "其他币种合计：" +
        curOrder.slice(1).map((c) => `${c} ${byCur[c]}`).join("；");
    } else {
      preview.hidden = true;
      preview.textContent = "";
    }
  }
}

async function markTaskCompleted(t) {
  const payload = taskToPayload(t);
  payload.status = "Done";
  payload.completedAt = todayLocalISO();
  try {
    await api(`/api/tasks/${encodeURIComponent(t.id)}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
    await loadTasks();
  } catch (err) {
    alert("更新失败: " + err.message);
  }
}

async function openTaskDialog(t) {
  lastOpenedTask = t || null;
  await ensurePricesLoaded();
  $("#dlg-task-title").textContent = t ? "编辑任务" : "新建任务";
  $("#task-id").value = t?.id || "";
  $("#task-no").value = t?.id || "";
  $("#task-company").value = t?.companyName || "";
  $("#task-date").value = t ? (t.date || "") : todayLocalISO();
  $("#task-status").value = t?.status === "Done" ? "Done" : "Pending";
  $("#task-note").value = t?.note || "";

  renderPriceCheckboxes(t?.selectedPriceIds || []);
  $("#task-s1").value = t?.service1 || "";
  $("#task-p1").value =
    t != null && t.price1 != null && !Number.isNaN(Number(t.price1)) ? t.price1 : "";
  $("#task-price-preview").hidden = true;
  $("#task-price-preview").textContent = "";
  dlgTask.showModal();
}

$("#task-cancel").addEventListener("click", () => dlgTask.close());

formTask.addEventListener("submit", async (e) => {
  e.preventDefault();
  const id = $("#task-id").value;
  const payload = {
    companyName: $("#task-company").value.trim(),
    date: $("#task-date").value,
    service1: $("#task-s1").value.trim(),
    service2: "",
    price1: parseFloat($("#task-p1").value) || 0,
    price2: 0,
    selectedPriceIds: getSelectedPriceIds(),
    status: $("#task-status").value,
    note: $("#task-note").value.trim(),
  };
  if (id && lastOpenedTask) {
    payload.completedAt = lastOpenedTask.completedAt || "";
  }
  try {
    if (id) {
      await api(`/api/tasks/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(payload) });
    } else {
      await api("/api/tasks", { method: "POST", body: JSON.stringify(payload) });
    }
    dlgTask.close();
    await loadTasks();
  } catch (err) {
    alert("保存失败: " + err.message);
  }
});

async function deleteTask(id) {
  if (!confirm("确定删除该任务？")) return;
  try {
    await api(`/api/tasks/${encodeURIComponent(id)}`, { method: "DELETE" });
    await loadTasks();
  } catch (err) {
    alert("删除失败: " + err.message);
  }
}

// --- Invoice ---
const dlgInvoice = document.getElementById("dlg-invoice");
const formInvoice = document.getElementById("form-invoice");

function addDaysISO(base, days) {
  const d = new Date(base);
  d.setDate(d.getDate() + days);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function openInvoiceDialog(task) {
  const today = todayLocalISO();
  document.getElementById("inv-task-id").value = task.id;
  document.getElementById("inv-bill-name").value = task.companyName || "";
  document.getElementById("inv-bill-addr").value = "";
  document.getElementById("inv-bill-email").value = "";
  document.getElementById("inv-ship-name").value = task.companyName || "";
  document.getElementById("inv-ship-addr").value = "";
  document.getElementById("inv-date").value = today;
  document.getElementById("inv-terms").value = "Net 30";
  document.getElementById("inv-due-date").value = addDaysISO(today, 30);
  document.getElementById("inv-currency").value = "USD";
  document.getElementById("inv-tax-rate").value = 0;
  document.getElementById("inv-desc").value = "Consulting Services";
  document.getElementById("inv-detail").value = task.service1 || "";
  document.getElementById("inv-qty").value = 1;
  document.getElementById("inv-rate").value = Number(task.price1 || 0);
  dlgInvoice.showModal();
}

document.getElementById("invoice-cancel")?.addEventListener("click", () => dlgInvoice.close());

formInvoice?.addEventListener("submit", async (e) => {
  e.preventDefault();
  const qty = parseFloat(document.getElementById("inv-qty").value) || 1;
  const rate = parseFloat(document.getElementById("inv-rate").value) || 0;
  const taxRate = parseFloat(document.getElementById("inv-tax-rate").value) || 0;
  const payload = {
    taskId: document.getElementById("inv-task-id").value,
    invoiceDate: document.getElementById("inv-date").value,
    terms: document.getElementById("inv-terms").value.trim(),
    dueDate: document.getElementById("inv-due-date").value,
    billToName: document.getElementById("inv-bill-name").value.trim(),
    billToAddr: document.getElementById("inv-bill-addr").value.trim(),
    billToEmail: document.getElementById("inv-bill-email").value.trim(),
    shipToName: document.getElementById("inv-ship-name").value.trim(),
    shipToAddr: document.getElementById("inv-ship-addr").value.trim(),
    currency: document.getElementById("inv-currency").value,
    taxRate,
    items: [
      {
        description: document.getElementById("inv-desc").value.trim(),
        detail: document.getElementById("inv-detail").value.trim(),
        taxLabel: taxRate === 0 ? "Zero-rated" : `GST @ ${taxRate}%`,
        qty,
        rate,
        amount: qty * rate,
      },
    ],
  };
  try {
    const created = await api("/api/invoices", { method: "POST", body: JSON.stringify(payload) });
    dlgInvoice.close();
    window.open(`/invoice.html?id=${encodeURIComponent(created.id)}`, "_blank");
  } catch (err) {
    alert("生成 Invoice 失败: " + err.message);
  }
});

// --- Invoices list / send / payment ---
let invoicesCache = [];
/** list | customers | payment */
let invoiceViewMode = "list";

async function loadInvoices() {
  const filter = document.getElementById("invoice-filter")?.value || "";
  try {
    if (invoiceViewMode === "customers" || invoiceViewMode === "payment") {
      invoicesCache = asArray(await api(`/api/invoices?status=`));
    } else {
      invoicesCache = asArray(await api(`/api/invoices?status=${encodeURIComponent(filter)}`));
    }
    if (invoiceViewMode === "customers") {
      renderCustomersList();
    } else {
      renderInvoices();
    }
  } catch (e) {
    console.error(e);
    alert("加载发票失败: " + e.message);
  }
}

function renderCustomersList() {
  const ul = document.getElementById("invoices-customers-list");
  const hint = document.getElementById("invoices-customers-hint");
  if (!ul) return;
  const names = new Set();
  for (const inv of asArray(invoicesCache)) {
    const n = (inv.billToName || "").trim();
    if (n) names.add(n);
  }
  const sorted = [...names].sort((a, b) => a.localeCompare(b));
  ul.innerHTML = "";
  for (const name of sorted) {
    const li = document.createElement("li");
    li.textContent = name;
    ul.appendChild(li);
  }
  if (hint) {
    hint.textContent = sorted.length
      ? `共 ${sorted.length} 个客户（Bill To）。`
      : "暂无客户数据，请先在「发票列表」中创建发票。";
  }
}

async function setInvoiceView(mode) {
  invoiceViewMode = mode;
  const listEl = document.getElementById("invoices-view-list");
  const custEl = document.getElementById("invoices-view-customers");
  const payEl = document.getElementById("invoices-view-payment");
  document.querySelectorAll(".invoices-nav-btn").forEach((btn) => {
    const v = btn.dataset.view;
    btn.classList.toggle("active", v === mode && mode !== "list");
  });
  if (listEl) listEl.hidden = mode !== "list";
  if (custEl) custEl.hidden = mode !== "customers";
  if (payEl) payEl.hidden = mode !== "payment";
  await loadInvoices();
}

function renderInvoices() {
  const mode = invoiceViewMode;
  const bodyId = mode === "payment" ? "invoices-body-payment" : "invoices-body";
  const body = document.getElementById(bodyId);
  const sum = document.getElementById(mode === "payment" ? "invoices-payment-summary" : "invoices-summary");
  if (!body || !sum) return;
  body.innerHTML = "";
  let invs = asArray(invoicesCache);
  if (mode === "payment") {
    invs = invs.filter((inv) => Number(inv.balanceDue) > 0);
  }
  sum.textContent =
    mode === "payment" ? `共 ${invs.length} 条待收款。` : `共 ${invs.length} 条发票。`;
  for (const inv of invs) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${escapeHtml(inv.invoiceNo || "")}</td>
      <td>${escapeHtml(inv.taskId || "")}</td>
      <td>${escapeHtml(inv.billToName || "")}</td>
      <td>${escapeHtml(inv.invoiceDate || "")}</td>
      <td>${escapeHtml(inv.dueDate || "")}</td>
      <td>${escapeHtml(inv.status || "")}</td>
      <td>${escapeHtml(fmtMoneySimple(inv.total, inv.currency))}</td>
      <td>${escapeHtml(fmtMoneySimple(inv.paidAmount, inv.currency))}</td>
      <td>${escapeHtml(fmtMoneySimple(inv.balanceDue, inv.currency))}</td>
      <td>${escapeHtml(inv.sentAt || "")}</td>
      <td class="row-actions">
        <button type="button" class="ghost" data-act="open">打开</button>
        <button type="button" class="ghost" data-act="send">Send</button>
        <button type="button" class="ghost success" data-act="pay">收款</button>
      </td>`;
    tr.querySelector('[data-act="open"]').addEventListener("click", () => {
      window.open(`/invoice.html?id=${encodeURIComponent(inv.id)}`, "_blank");
    });
    tr.querySelector('[data-act="send"]').addEventListener("click", () => openSendInvoice(inv.id));
    tr.querySelector('[data-act="pay"]').addEventListener("click", () => openPayDialog(inv.id));
    body.appendChild(tr);
  }
}

function fmtMoneySimple(v, c) {
  const n = Number(v || 0);
  const cur = c || "";
  return (cur ? cur + " " : "") + n.toLocaleString("en-CA", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

document.getElementById("btn-invoices-load")?.addEventListener("click", () => loadInvoices());
document.getElementById("invoice-filter")?.addEventListener("change", () => loadInvoices());

document.querySelectorAll(".invoices-nav-btn").forEach((btn) => {
  btn.addEventListener("click", () => {
    if (btn.dataset.action === "new-invoice") {
      openNewInvoiceFromMenu();
      return;
    }
    const v = btn.dataset.view;
    if (v === "customers" || v === "payment") {
      setInvoiceView(v);
    }
  });
});

document.getElementById("btn-invoices-back-customers")?.addEventListener("click", () => setInvoiceView("list"));
document.getElementById("btn-invoices-back-payment")?.addEventListener("click", () => setInvoiceView("list"));

const dlgPickTask = document.getElementById("dlg-pick-task");
const formPickTask = document.getElementById("form-pick-task");

async function openNewInvoiceFromMenu() {
  try {
    await loadTasks();
  } catch {
    /* loadTasks already alerts */
  }
  const sel = document.getElementById("pick-task-id");
  if (!sel || !dlgPickTask) return;
  const tasks = asArray(tasksCache);
  sel.innerHTML = "";
  if (tasks.length === 0) {
    alert("暂无任务，请先在「任务」页创建任务。");
    return;
  }
  for (const t of tasks) {
    const opt = document.createElement("option");
    opt.value = t.id;
    opt.textContent = `${t.id} — ${t.companyName || ""}`;
    sel.appendChild(opt);
  }
  dlgPickTask.showModal();
}

document.getElementById("pick-task-cancel")?.addEventListener("click", () => dlgPickTask?.close());

formPickTask?.addEventListener("submit", (e) => {
  e.preventDefault();
  const id = document.getElementById("pick-task-id")?.value;
  const task = asArray(tasksCache).find((t) => t.id === id);
  if (!task) {
    alert("请选择任务");
    return;
  }
  dlgPickTask?.close();
  openInvoiceDialog(task);
});

async function openSendInvoice(id) {
  const to = prompt("发送到邮箱（Bill To Email）：");
  if (!to) return;
  try {
    await api(`/api/invoices/${encodeURIComponent(id)}/send`, { method: "POST", body: JSON.stringify({ to }) });
    alert("已发送");
    await loadInvoices();
    await loadTasks(); // 同步任务状态为 Sent
  } catch (e) {
    alert("发送失败: " + e.message);
  }
}

const dlgPay = document.getElementById("dlg-pay");
const formPay = document.getElementById("form-pay");

function openPayDialog(invoiceId) {
  document.getElementById("pay-invoice-id").value = invoiceId;
  document.getElementById("pay-date").value = todayLocalISO();
  document.getElementById("pay-amount").value = "";
  dlgPay.showModal();
}

document.getElementById("pay-cancel")?.addEventListener("click", () => dlgPay.close());

formPay?.addEventListener("submit", async (e) => {
  e.preventDefault();
  const id = document.getElementById("pay-invoice-id").value;
  const date = document.getElementById("pay-date").value;
  const amount = parseFloat(document.getElementById("pay-amount").value);
  if (!amount || amount <= 0) {
    alert("请输入收款金额");
    return;
  }
  try {
    await api(`/api/invoices/${encodeURIComponent(id)}/payment`, { method: "POST", body: JSON.stringify({ amount, date }) });
    dlgPay.close();
    await loadInvoices();
    await loadTasks(); // 若 fully paid，会把 task 状态改为 Paid
  } catch (err) {
    alert("保存失败: " + err.message);
  }
});

// --- Prices ---
let pricesCache = [];

async function loadPrices() {
  try {
    pricesCache = asArray(await api("/api/prices"));
    renderPrices();
  } catch (e) {
    console.error(e);
    alert("加载价目失败: " + e.message);
  }
}

function curLabel(c) {
  if (c === "CNY") return "元";
  if (c === "CAD") return "加币";
  if (c === "USD") return "刀";
  return c || "";
}

function renderPrices() {
  const body = $("#prices-body");
  body.innerHTML = "";
  for (const p of asArray(pricesCache)) {
    const tr = document.createElement("tr");
    const amt =
      p.amount != null && p.amount !== undefined
        ? String(p.amount)
        : "—";
    tr.innerHTML = `
      <td>${escapeHtml(p.id)}</td>
      <td>${escapeHtml(p.serviceName)}</td>
      <td>${escapeHtml(amt)}</td>
      <td>${escapeHtml(p.currency)} ${curLabel(p.currency)}</td>
      <td>${escapeHtml(p.note || "")}</td>
      <td class="row-actions">
        <button type="button" class="ghost" data-act="edit">编辑</button>
        <button type="button" class="ghost danger" data-act="del">删除</button>
      </td>`;
    tr.querySelector('[data-act="edit"]').addEventListener("click", () => openPriceDialog(p));
    tr.querySelector('[data-act="del"]').addEventListener("click", () => deletePrice(p.id));
    body.appendChild(tr);
  }
}

$("#btn-new-price").addEventListener("click", () => openPriceDialog(null));

const dlgPrice = $("#dlg-price");
const formPrice = $("#form-price");

function openPriceDialog(p) {
  $("#dlg-price-title").textContent = p ? "编辑价目" : "新建价目";
  $("#price-id").value = p?.id || "";
  $("#price-name").value = p?.serviceName || "";
  $("#price-amount").value =
    p && p.amount != null && p.amount !== undefined ? p.amount : "";
  $("#price-currency").value = p?.currency || "CNY";
  $("#price-note").value = p?.note || "";
  dlgPrice.showModal();
}

$("#price-cancel").addEventListener("click", () => dlgPrice.close());

formPrice.addEventListener("submit", async (e) => {
  e.preventDefault();
  const id = $("#price-id").value;
  const rawAmt = $("#price-amount").value.trim();
  const payload = {
    serviceName: $("#price-name").value.trim(),
    currency: $("#price-currency").value,
    note: $("#price-note").value.trim(),
  };
  if (rawAmt === "") {
    payload.amount = null;
  } else {
    payload.amount = parseFloat(rawAmt);
  }
  try {
    if (id) {
      await api(`/api/prices/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(payload) });
    } else {
      await api("/api/prices", { method: "POST", body: JSON.stringify(payload) });
    }
    dlgPrice.close();
    await loadPrices();
  } catch (err) {
    alert("保存失败: " + err.message);
  }
});

async function deletePrice(id) {
  if (!confirm("确定删除该价目？")) return;
  try {
    await api(`/api/prices/${encodeURIComponent(id)}`, { method: "DELETE" });
    await loadPrices();
  } catch (err) {
    alert("删除失败: " + err.message);
  }
}

function escapeHtml(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

function fmtNum(n) {
  if (n == null || Number.isNaN(n)) return "";
  return Number(n).toLocaleString("zh-CN", { maximumFractionDigits: 2 });
}

// --- Report（按月查看已完成任务 + 导出 CSV）---
let reportRows = [];

async function loadReport() {
  const monthEl = document.getElementById("report-month");
  const sumEl = document.getElementById("report-summary");
  const bodyEl = document.getElementById("report-body");
  if (!monthEl || !sumEl || !bodyEl) return;
  const month = monthEl.value;
  if (!month) {
    sumEl.textContent = "请选择月份。";
    bodyEl.innerHTML = "";
    reportRows = [];
    return;
  }
  try {
    reportRows = asArray(await api(`/api/reports/completed?month=${encodeURIComponent(month)}`));
    sumEl.textContent = `${month} 共完成 ${reportRows.length} 条任务（状态为 Done，且完成日期在该月内）。`;
    bodyEl.innerHTML = "";
    for (const t of reportRows) {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td>${escapeHtml(t.id)}</td>
        <td>${escapeHtml(t.companyName)}</td>
        <td>${escapeHtml(t.date || "")}</td>
        <td>${escapeHtml(t.service1 || "")}</td>
        <td>${fmtNum(t.price1)}</td>
        <td>${escapeHtml(t.completedAt || "")}</td>
        <td>${escapeHtml(t.note || "")}</td>`;
      bodyEl.appendChild(tr);
    }
  } catch (e) {
    console.error(e);
    alert("加载报表失败: " + e.message);
  }
}

function csvEscape(cell) {
  const s = String(cell ?? "");
  if (/[",\n\r]/.test(s)) {
    return '"' + s.replace(/"/g, '""') + '"';
  }
  return s;
}

function exportReportCSV() {
  const month = document.getElementById("report-month")?.value;
  if (!month) {
    alert("请先选择月份并查询。");
    return;
  }
  const rows = asArray(reportRows);
  if (rows.length === 0) {
    alert("当前没有可导出的数据，请先查询。");
    return;
  }
  const headers = ["No.", "公司名", "日期", "业务一", "价格一", "状态", "完成日期", "备注"];
  const lines = [headers.join(",")];
  for (const t of rows) {
    lines.push(
      [
        csvEscape(t.id),
        csvEscape(t.companyName),
        csvEscape(t.date),
        csvEscape(t.service1),
        csvEscape(t.price1),
        csvEscape(t.status),
        csvEscape(t.completedAt),
        csvEscape(t.note),
      ].join(","),
    );
  }
  const csv = "\uFEFF" + lines.join("\r\n");
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = `completed-report-${month}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
}

document.getElementById("btn-report-load")?.addEventListener("click", () => loadReport());
document.getElementById("btn-report-export")?.addEventListener("click", () => exportReportCSV());

(async function init() {
  let me;
  try {
    me = await fetch("/api/me", { credentials: "same-origin" }).then((r) => r.json());
  } catch {
    loadTasks();
    return;
  }
  const btn = document.getElementById("btn-logout");
  if (btn) {
    btn.hidden = !me.authEnabled;
    btn.addEventListener("click", async () => {
      try {
        await fetch("/api/logout", { method: "POST", credentials: "same-origin" });
      } catch {
        /* ignore */
      }
      window.location.href = "/login.html";
    });
  }
  if (me.authEnabled && me.needsSetup) {
    window.location.href = "/setup.html";
    return;
  }
  if (me.authEnabled && !me.authenticated) {
    window.location.href = "/login.html";
    return;
  }
  try {
    const pub = await fetch("/api/settings/public").then((r) => r.json());
    const title = document.getElementById("app-title");
    if (pub.companyName && title) title.textContent = pub.companyName;
    if (pub.logoDataUrl) {
      const logo = document.getElementById("app-header-logo");
      if (logo) {
        logo.innerHTML = "";
        const img = document.createElement("img");
        img.src = pub.logoDataUrl;
        img.alt = "";
        logo.appendChild(img);
        logo.hidden = false;
      }
    }
  } catch {
    /* ignore */
  }
  const rm = document.getElementById("report-month");
  if (rm) rm.value = currentMonthISO();
  loadTasks();
})();
