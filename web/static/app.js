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
    if (tab === "trend") loadTrend();
    if (tab === "invoices") loadInvoices();
    if (tab === "report") {
      const rm = document.getElementById("report-month");
      if (rm && !rm.value) rm.value = currentMonthISO();
      loadReport();
    }
    if (tab === "prices") loadPrices();
    if (tab === "expense") loadExpenses();
    if (tab === "exchange") loadExchangeTab().catch((e) => console.error(e));
    if (tab === "cheque") {
      const fr = document.getElementById("iframe-cheque");
      if (fr && !fr.getAttribute("src")) {
        fr.src = "/check.html?embed=1";
      }
    }
  });
});

// --- Tasks ---
let tasksCache = [];
let customersCache = [];
/** 当前编辑对话框打开时的任务快照（用于保留 completedAt 等未在表单展示的字段） */
let lastOpenedTask = null;

function customerIsActive(c) {
  const s = String((c && c.status) || "active").toLowerCase();
  return s !== "inactive";
}

function taskCustomerStatusActive(t) {
  const s = String((t && t.customerStatus) || "active").toLowerCase();
  return s !== "inactive";
}

async function loadCustomers() {
  customersCache = asArray(await api("/api/customers"));
}

async function ensureCustomersLoaded() {
  try {
    await loadCustomers();
  } catch (e) {
    console.error(e);
    alert("加载客户列表失败: " + e.message);
  }
}

function fillTaskCustomerSelect(selectedId) {
  const sel = document.getElementById("task-customer");
  if (!sel) return;
  sel.innerHTML = "";
  const empty = document.createElement("option");
  empty.value = "";
  empty.textContent = "请选择 Customer";
  sel.appendChild(empty);
  const sorted = [...customersCache].sort((a, b) => String(a.id).localeCompare(String(b.id)));
  for (const c of sorted) {
    if (!customerIsActive(c) && String(c.id) !== String(selectedId || "")) {
      continue;
    }
    const o = document.createElement("option");
    o.value = c.id;
    o.textContent = (c.name || c.id).trim() || c.id;
    sel.appendChild(o);
  }
  if (selectedId) sel.value = selectedId;
}

function taskToPayload(t) {
  return {
    customerId: t.customerId || "",
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
  const rows = asArray(tasksCache)
    .filter((t) => t.status !== "Paid")
    .filter((t) => !filter || t.status === filter)
    .sort((a, b) => {
      const pa = a.status === "Pending" ? 0 : 1;
      const pb = b.status === "Pending" ? 0 : 1;
      if (pa !== pb) return pa - pb;
      return String(a.id).localeCompare(String(b.id));
    });
  for (const t of rows) {
    const tr = document.createElement("tr");
    const done = t.status === "Done";
    const canDelete = t.status === "Pending";
    const cust = escapeHtml(t.customerName || "");
    const cn = escapeHtml(t.companyName || "");
    tr.innerHTML = `
      <td>${escapeHtml(t.id)}</td>
      <td>${cust}</td>
      <td>${cn}</td>
      <td>${escapeHtml(t.date || "")}</td>
      <td>${escapeHtml(t.service1 || "")}</td>
      <td>${fmtNum(t.price1)}</td>
      <td><span class="${done ? "status-done" : "status-pending"}">${escapeHtml(t.status)}</span></td>
      <td>${escapeHtml(t.completedAt || "")}</td>
      <td>${escapeHtml(t.note || "")}</td>
      <td class="row-actions">
        <button type="button" class="ghost" data-act="edit">编辑</button>
        <button type="button" class="primary-green" data-act="expense">Expense</button>
        <button type="button" class="ghost success" data-act="done" ${done ? "disabled" : ""}>Completed</button>
        <button type="button" class="ghost danger" data-act="del" ${canDelete ? "" : "disabled"}>删除</button>
      </td>`;
    tr.querySelector('[data-act="edit"]').addEventListener("click", () => openTaskDialog(t));
    const btnDone = tr.querySelector('[data-act="done"]');
    if (!done) {
      btnDone.addEventListener("click", () => markTaskCompleted(t));
    }
    tr.querySelector('[data-act="expense"]').addEventListener("click", () => openNewExpenseForTask(t).catch((e) => alert(e.message)));
    tr.querySelector('[data-act="del"]').addEventListener("click", () => deleteTask(t.id));
    body.appendChild(tr);
  }
}

$("#filter-status").addEventListener("change", renderTasks);

$("#btn-new-task").addEventListener("click", () => openTaskDialog(null));

document.getElementById("btn-new-customer")?.addEventListener("click", async () => {
  const name = prompt("新客户名称（将出现在 Customer 列表中）：");
  if (!name || !String(name).trim()) return;
  try {
    const c = await api("/api/customers", { method: "POST", body: JSON.stringify({ name: String(name).trim() }) });
    await loadCustomers();
    fillTaskCustomerSelect(c.id);
  } catch (e) {
    alert("添加失败: " + e.message);
  }
});

const dlgTask = $("#dlg-task");
const formTask = $("#form-task");
const taskPricePicks = $("#task-price-picks");
/** 从 Invoices 打开任务编辑时为 true，可改 Done/Sent */
let taskDialogInvoiceEdit = false;

function setTaskFormLocked(locked) {
  const hint = document.getElementById("task-lock-hint");
  const submit = document.getElementById("task-submit-btn");
  const clearBtn = document.getElementById("task-price-clear");
  if (submit) submit.disabled = locked;
  if (clearBtn) clearBtn.disabled = locked;
  if (hint) {
    hint.hidden = !locked;
    hint.textContent = "此任务在任务页已锁定。Done / Sent 请在 Invoices 中修改；Paid 不可修改。";
  }
  document.querySelectorAll("#task-price-picks input[type=checkbox]").forEach((cb) => {
    cb.disabled = locked;
  });
  document.querySelectorAll("#form-task input, #form-task textarea, #form-task select").forEach((el) => {
    if (el.id === "task-id") return;
    if (el.type === "hidden") return;
    el.disabled = locked;
  });
  const bn = document.getElementById("btn-new-customer");
  if (bn) bn.disabled = locked;
}

dlgTask.addEventListener("close", () => {
  taskDialogInvoiceEdit = false;
  setTaskFormLocked(false);
});

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

/** 根据当前勾选同步 Task / Price（勾选变化时调用；多币种时其余写在预览区） */
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

async function openTaskDialog(t, opts = {}) {
  if (t && t.status === "Paid") {
    alert("Paid 任务不可修改。");
    return;
  }
  taskDialogInvoiceEdit = !!(opts && opts.invoiceEdit);
  const locked =
    !!(t && (t.status === "Done" || t.status === "Sent" || t.status === "Paid") && !taskDialogInvoiceEdit);
  lastOpenedTask = t || null;
  await ensureCustomersLoaded();
  await ensurePricesLoaded();
  setTaskFormLocked(false);
  $("#dlg-task-title").textContent = t ? "编辑任务" : "新建任务";
  $("#task-id").value = t?.id || "";
  fillTaskCustomerSelect(t?.customerId || "");
  const hintNew = document.getElementById("task-no-new-hint");
  const wrapEdit = document.getElementById("task-no-edit-wrap");
  const valNo = document.getElementById("task-no-value");
  const isNew = !t;
  if (hintNew) hintNew.hidden = !isNew;
  if (wrapEdit) wrapEdit.hidden = isNew;
  if (valNo && t) valNo.textContent = t.id;
  $("#task-company").value = t?.companyName || "";
  $("#task-date").value = t ? (t.date || "") : todayLocalISO();
  $("#task-status").value = t?.status || "Pending";
  $("#task-note").value = t?.note || "";

  renderPriceCheckboxes(t?.selectedPriceIds || []);
  $("#task-s1").value = t?.service1 || "";
  $("#task-p1").value =
    t != null && t.price1 != null && !Number.isNaN(Number(t.price1)) ? t.price1 : "";
  $("#task-price-preview").hidden = true;
  $("#task-price-preview").textContent = "";
  dlgTask.showModal();
  setTaskFormLocked(locked);
}

$("#task-cancel").addEventListener("click", () => dlgTask.close());

formTask.addEventListener("submit", async (e) => {
  e.preventDefault();
  const id = $("#task-id").value;
  const submitBtn = document.getElementById("task-submit-btn");
  if (submitBtn && submitBtn.disabled) return;
  const payload = {
    customerId: ($("#task-customer") && $("#task-customer").value) || "",
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
    if (taskDialogInvoiceEdit) {
      payload.status = lastOpenedTask.status;
    }
  }
  try {
    if (id) {
      const q = taskDialogInvoiceEdit ? "?invoiceEdit=1" : "";
      await api(`/api/tasks/${encodeURIComponent(id)}${q}`, { method: "PUT", body: JSON.stringify(payload) });
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
    alert("删除失败: " + (err.message || err));
  }
}

// --- Invoice ---
const dlgInvoice = document.getElementById("dlg-invoice");
const formInvoice = document.getElementById("form-invoice");
/** 合并开票时非空（多任务）；单任务开票为 null */
let invoiceMultiTasks = null;

dlgInvoice?.addEventListener("close", () => {
  invoiceMultiTasks = null;
  setInvoiceDialogMode(false);
});

function addDaysISO(base, days) {
  const d = new Date(base);
  d.setDate(d.getDate() + days);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function setInvoiceDialogMode(isMulti) {
  const singleEl = document.getElementById("inv-single-line-fields");
  const multiEl = document.getElementById("inv-multi-line-block");
  if (singleEl) singleEl.hidden = !!isMulti;
  if (multiEl) multiEl.hidden = !isMulti;
}

function openInvoiceDialog(task) {
  invoiceMultiTasks = null;
  setInvoiceDialogMode(false);
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

/** 同一客户的多条 Done 任务合并开票 */
function openInvoiceDialogMulti(tasks) {
  if (!tasks || tasks.length === 0) return;
  if (tasks.length === 1) {
    openInvoiceDialog(tasks[0]);
    return;
  }
  invoiceMultiTasks = tasks;
  setInvoiceDialogMode(true);
  const first = tasks[0];
  const today = todayLocalISO();
  document.getElementById("inv-task-id").value = first.id;
  document.getElementById("inv-bill-name").value = first.companyName || "";
  document.getElementById("inv-bill-addr").value = "";
  document.getElementById("inv-bill-email").value = "";
  document.getElementById("inv-ship-name").value = first.companyName || "";
  document.getElementById("inv-ship-addr").value = "";
  document.getElementById("inv-date").value = today;
  document.getElementById("inv-terms").value = "Net 30";
  document.getElementById("inv-due-date").value = addDaysISO(today, 30);
  document.getElementById("inv-currency").value = "USD";
  document.getElementById("inv-tax-rate").value = 0;
  const tbody = document.getElementById("inv-multi-body");
  if (tbody) {
    tbody.innerHTML = "";
    for (const t of tasks) {
      const rate = Number(t.price1) || 0;
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td>${escapeHtml(t.id)}</td>
        <td>${escapeHtml(t.service1 || "")}</td>
        <td>1</td>
        <td>${escapeHtml(String(rate))}</td>
        <td>${escapeHtml(String(rate))}</td>`;
      tbody.appendChild(tr);
    }
  }
  dlgInvoice.showModal();
}

document.getElementById("invoice-cancel")?.addEventListener("click", () => dlgInvoice.close());

formInvoice?.addEventListener("submit", async (e) => {
  e.preventDefault();
  const taxRate = parseFloat(document.getElementById("inv-tax-rate").value) || 0;
  const taxLabel = taxRate === 0 ? "Zero-rated" : `GST @ ${taxRate}%`;
  let items;
  let taskIds;
  if (invoiceMultiTasks && invoiceMultiTasks.length > 1) {
    taskIds = invoiceMultiTasks.map((t) => t.id);
    items = invoiceMultiTasks.map((t) => {
      const rate = Number(t.price1) || 0;
      return {
        description: "Consulting Services",
        detail: (t.service1 || "").trim(),
        taxLabel,
        qty: 1,
        rate,
        amount: rate,
      };
    });
  } else {
    const qty = parseFloat(document.getElementById("inv-qty").value) || 1;
    const rate = parseFloat(document.getElementById("inv-rate").value) || 0;
    taskIds = null;
    items = [
      {
        description: document.getElementById("inv-desc").value.trim(),
        detail: document.getElementById("inv-detail").value.trim(),
        taxLabel,
        qty,
        rate,
        amount: qty * rate,
      },
    ];
  }
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
    items,
  };
  if (taskIds && taskIds.length > 1) {
    payload.taskIds = taskIds;
  }
  try {
    const created = await api("/api/invoices", { method: "POST", body: JSON.stringify(payload) });
    invoiceMultiTasks = null;
    dlgInvoice.close();
    window.open(`/invoice.html?id=${encodeURIComponent(created.id)}`, "_blank");
    if (invoiceViewMode === "new-invoice") {
      await loadTasks();
      renderNewInvoiceView();
    }
  } catch (err) {
    alert("生成 Invoice 失败: " + err.message);
  }
});

// --- Invoices list / send / payment ---
let invoicesCache = [];
/** list | customers | payment | new-invoice */
let invoiceViewMode = "list";

async function loadInvoices() {
  if (invoiceViewMode === "new-invoice") {
    try {
      await loadTasks();
      renderNewInvoiceView();
    } catch (e) {
      console.error(e);
      alert("加载任务失败: " + e.message);
    }
    return;
  }
  if (invoiceViewMode === "customers") {
    try {
      await loadCustomers();
      renderCustomersList();
    } catch (e) {
      console.error(e);
      alert("加载客户失败: " + e.message);
    }
    return;
  }
  const filter = document.getElementById("invoice-filter")?.value || "";
  try {
    if (invoiceViewMode === "payment") {
      invoicesCache = asArray(await api(`/api/invoices?status=`));
    } else {
      invoicesCache = asArray(await api(`/api/invoices?status=${encodeURIComponent(filter)}`));
    }
    renderInvoices();
  } catch (e) {
    console.error(e);
    alert("加载发票失败: " + e.message);
  }
}

function renderCustomersList() {
  const tbody = document.getElementById("invoices-customers-list");
  const hint = document.getElementById("invoices-customers-hint");
  if (!tbody) return;
  const rows = [...asArray(customersCache)].sort((a, b) => String(a.id).localeCompare(String(b.id)));
  tbody.innerHTML = "";
  for (const c of rows) {
    const active = customerIsActive(c);
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${escapeHtml(c.id)}</td>
      <td>${escapeHtml((c.name || "").trim() || c.id)}</td>
      <td>${active ? `<span class="status-done">active</span>` : `<span class="status-pending">inactive</span>`}</td>
      <td class="row-actions">
        <button type="button" class="ghost small" data-act="edit">Edit</button>
      </td>`;
    tr.querySelector('[data-act="edit"]').addEventListener("click", () => openCustomerEditDialog(c.id));
    tbody.appendChild(tr);
  }
  if (hint) {
    hint.textContent = rows.length
      ? `共 ${rows.length} 个客户。`
      : "暂无客户，请点击右下角 New Customer 添加。";
  }
}

async function setInvoiceView(mode) {
  invoiceViewMode = mode;
  const listEl = document.getElementById("invoices-view-list");
  const custEl = document.getElementById("invoices-view-customers");
  const payEl = document.getElementById("invoices-view-payment");
  const newEl = document.getElementById("invoices-view-new-invoice");
  document.querySelectorAll(".invoices-nav-btn").forEach((btn) => {
    const v = btn.dataset.view;
    const isNew = btn.dataset.action === "new-invoice";
    const active = (v === mode && mode !== "list") || (isNew && mode === "new-invoice");
    btn.classList.toggle("active", active);
  });
  if (listEl) listEl.hidden = mode !== "list";
  if (custEl) custEl.hidden = mode !== "customers";
  if (payEl) payEl.hidden = mode !== "payment";
  if (newEl) newEl.hidden = mode !== "new-invoice";
  await loadInvoices();
}

function getDoneTasksForInvoice() {
  return asArray(tasksCache)
    .filter((t) => t.status === "Done")
    .filter((t) => taskCustomerStatusActive(t))
    .sort((a, b) => {
      const ca = (a.customerName || "").trim();
      const cb = (b.customerName || "").trim();
      if (ca !== cb) return ca.localeCompare(cb);
      const ga = (a.companyName || "").trim();
      const gb = (b.companyName || "").trim();
      if (ga !== gb) return ga.localeCompare(gb);
      return String(a.id).localeCompare(String(b.id));
    });
}

function updateInvoicingButtonState() {
  const btn = document.getElementById("btn-invoicing-go");
  if (!btn) return;
  const selected = [...document.querySelectorAll(".new-inv-cb:checked")]
    .map((x) => tasksCache.find((t) => t.id === x.dataset.taskId))
    .filter(Boolean);
  if (selected.length === 0) {
    btn.disabled = true;
    return;
  }
  const custIds = new Set(selected.map((t) => (t.customerId || "").trim()));
  btn.disabled = custIds.size !== 1 || [...custIds][0] === "";
}

function onNewInvoiceCheckboxChange(e) {
  const cb = e.target;
  if (!cb.classList.contains("new-inv-cb")) return;
  if (!cb.checked) {
    updateInvoicingButtonState();
    return;
  }
  const selected = [...document.querySelectorAll(".new-inv-cb:checked")]
    .map((x) => tasksCache.find((t) => t.id === x.dataset.taskId))
    .filter(Boolean);
  const custIds = new Set(selected.map((t) => (t.customerId || "").trim()));
  if (custIds.size > 1) {
    alert("只能选择同一 Customer 的任务。");
    cb.checked = false;
  }
  updateInvoicingButtonState();
}

function renderNewInvoiceView() {
  const tbody = document.getElementById("new-invoice-body");
  const sum = document.getElementById("new-invoice-summary");
  if (!tbody) return;
  const done = getDoneTasksForInvoice();
  tbody.innerHTML = "";
  if (sum) {
    sum.textContent =
      done.length > 0
        ? `共 ${done.length} 条可开票任务（状态为 Done）。`
        : "暂无可开票任务：请先在「任务」页将任务标记为 Done。";
  }
  for (const t of done) {
    const tr = document.createElement("tr");
    const doneCls = t.status === "Done";
    tr.innerHTML = `
      <td><input type="checkbox" class="new-inv-cb" data-task-id="${escapeHtml(t.id)}" /></td>
      <td>${escapeHtml(t.id)}</td>
      <td>${escapeHtml(t.customerName || "")}</td>
      <td>${escapeHtml(t.companyName || "")}</td>
      <td>${escapeHtml(t.date || "")}</td>
      <td>${escapeHtml(t.service1 || "")}</td>
      <td>${fmtNum(t.price1)}</td>
      <td><span class="${doneCls ? "status-done" : "status-pending"}">${escapeHtml(t.status)}</span></td>
      <td class="row-actions">
        <button type="button" class="ghost small" data-act="edit-from-inv">编辑</button>
      </td>`;
    tr.querySelector(".new-inv-cb").addEventListener("change", onNewInvoiceCheckboxChange);
    tr.querySelector('[data-act="edit-from-inv"]').addEventListener("click", () =>
      openTaskDialog(t, { invoiceEdit: true }),
    );
    tbody.appendChild(tr);
  }
  updateInvoicingButtonState();
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
        ${
          Number(inv.balanceDue) > 0
            ? `<button type="button" class="ghost small" data-act="check">支票</button>`
            : ""
        }
        <button type="button" class="ghost small" data-act="edit-task">编辑任务</button>
      </td>`;
    tr.querySelector('[data-act="open"]').addEventListener("click", () => {
      window.open(`/invoice.html?id=${encodeURIComponent(inv.id)}`, "_blank");
    });
    tr.querySelector('[data-act="send"]').addEventListener("click", () => openSendInvoice(inv.id));
    tr.querySelector('[data-act="pay"]').addEventListener("click", () => openPayDialog(inv.id));
    const chkBtn = tr.querySelector('[data-act="check"]');
    if (chkBtn) {
      chkBtn.addEventListener("click", () => {
        window.open(`/check.html?id=${encodeURIComponent(inv.id)}`, "_blank");
      });
    }
    tr.querySelector('[data-act="edit-task"]').addEventListener("click", async () => {
      const tid = (inv.taskId || "").trim();
      if (!tid) return;
      try {
        const task = await api(`/api/tasks/${encodeURIComponent(tid)}`);
        if (task.status === "Paid") {
          alert("Paid 任务不可修改。");
          return;
        }
        const ie = task.status === "Done" || task.status === "Sent";
        openTaskDialog(task, { invoiceEdit: ie });
      } catch (err) {
        alert("加载任务失败: " + err.message);
      }
    });
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
      setInvoiceView("new-invoice");
      return;
    }
    const v = btn.dataset.view;
    if (v === "customers" || v === "payment") {
      setInvoiceView(v);
    }
  });
});

document.getElementById("btn-invoices-back-customers")?.addEventListener("click", () => setInvoiceView("list"));

document.getElementById("btn-invoices-new-customer")?.addEventListener("click", async () => {
  const name = prompt("新客户名称：");
  if (!name || !String(name).trim()) return;
  try {
    await api("/api/customers", { method: "POST", body: JSON.stringify({ name: String(name).trim() }) });
    await loadCustomers();
    if (invoiceViewMode === "customers") {
      renderCustomersList();
    }
  } catch (e) {
    alert("添加失败: " + e.message);
  }
});

const dlgCustomer = document.getElementById("dlg-customer");
const formCustomer = document.getElementById("form-customer");
const phoneFormatRx = /^\+?\d{10,15}$/;
const emailFormatRx = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

// --- Trend ---
let trendPieTasksChart = null;
let trendPieAmountChart = null;
let trendPiePendingChart = null;
let trendBarMonthlyChart = null;

function destroyChart(ch) {
  if (ch && typeof ch.destroy === "function") {
    ch.destroy();
  }
}

function renderDoughnutChart(ctx, value, total, label) {
  const v = Number(value) || 0;
  const t = Number(total) || 0;
  const done = Math.max(0, Math.min(v, t));
  const rest = Math.max(0, t - done);
  return new Chart(ctx, {
    type: "doughnut",
    data: {
      labels: [label, "其他"],
      datasets: [
        {
          data: [done, rest],
          backgroundColor: ["#3d8bfd", "#1f2933"],
          borderWidth: 0,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      cutout: "60%",
      layout: {
        padding: { top: 4, bottom: 4, left: 4, right: 4 },
      },
      plugins: {
        legend: {
          display: false,
        },
      },
    },
  });
}

async function loadTrend() {
  const month = currentMonthISO();
  try {
    const data = await api(`/api/reports/trend?month=${encodeURIComponent(month)}`);
    const monthLabel = (data && data.month) || month;
    // 更新文字
    document.getElementById("trend-tasks-done").textContent = data.monthlyTasksDone ?? 0;
    document.getElementById("trend-tasks-total").textContent = data.monthlyTasksTotal ?? 0;
    document.getElementById("trend-amount-done").textContent = (data.monthlyAmountDone || 0).toFixed(2);
    document.getElementById("trend-amount-total").textContent = (data.monthlyAmountTotal || 0).toFixed(2);
    document.getElementById("trend-pending-new").textContent = (data.pendingAmountNewThisMonth || 0).toFixed(2);
    document.getElementById("trend-pending-total").textContent = (data.pendingAmountTotal || 0).toFixed(2);
    document.getElementById("trend-invoiced-amount").textContent = `CAD ${(data.monthlyInvoicedAmount || 0).toFixed(2)}`;
    document.getElementById("trend-invoiced-month-label").textContent = `${monthLabel} 本月开票总额`;

    // 圆图
    const ctxTasks = document.getElementById("trend-pie-tasks")?.getContext("2d");
    const ctxAmount = document.getElementById("trend-pie-amount")?.getContext("2d");
    const ctxPending = document.getElementById("trend-pie-pending")?.getContext("2d");
    if (ctxTasks && ctxAmount && ctxPending && window.Chart) {
      destroyChart(trendPieTasksChart);
      destroyChart(trendPieAmountChart);
      destroyChart(trendPiePendingChart);
      trendPieTasksChart = renderDoughnutChart(
        ctxTasks,
        data.monthlyTasksDone || 0,
        data.monthlyTasksTotal || 0,
        "已完成任务",
      );
      trendPieAmountChart = renderDoughnutChart(
        ctxAmount,
        data.monthlyAmountDone || 0,
        data.monthlyAmountTotal || 0,
        "已完成金额",
      );
      trendPiePendingChart = renderDoughnutChart(
        ctxPending,
        data.pendingAmountNewThisMonth || 0,
        data.pendingAmountTotal || 0,
        "本月新增 Pending",
      );
    }

    // 柱状图
    const barCtx = document.getElementById("trend-bar-monthly")?.getContext("2d");
    if (barCtx && window.Chart && Array.isArray(data.monthlySeries)) {
      const labels = data.monthlySeries.map((p) => p.month);
      const tasks = data.monthlySeries.map((p) => p.tasksNew || 0);
      const amounts = data.monthlySeries.map((p) => p.amountNew || 0);
      destroyChart(trendBarMonthlyChart);
      trendBarMonthlyChart = new Chart(barCtx, {
        type: "bar",
        data: {
          labels,
          datasets: [
            {
              label: "新增金额 (CAD)",
              data: amounts,
              backgroundColor: "rgba(126, 91, 239, 0.8)",
            },
          ],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          layout: {
            padding: { top: 18, bottom: 0, left: 8, right: 8 },
          },
          scales: {
            x: {
              ticks: { color: "#8b9cb3", maxRotation: 45, minRotation: 0, font: { size: 10 } },
              grid: { display: false },
            },
            y: {
              beginAtZero: true,
              ticks: { color: "#8b9cb3", maxTicksLimit: 6, font: { size: 10 } },
              grid: { color: "rgba(148, 163, 184, 0.25)" },
            },
          },
          plugins: {
            legend: {
              position: "top",
              align: "end",
              labels: { color: "#e8edf4", boxWidth: 12, font: { size: 11 } },
            },
          },
        },
        plugins: [
          {
            id: "trendMonthlyTaskCountInBar",
            afterDatasetsDraw(chart) {
              const ctx = chart.ctx;
              const meta = chart.getDatasetMeta(0);
              if (!meta?.data?.length) return;
              ctx.save();
              ctx.textAlign = "center";
              meta.data.forEach((el, i) => {
                const n = tasks[i];
                const { x, y, base } = el.getProps(["x", "y", "base"], true);
                const top = Math.min(y, base);
                const bottom = Math.max(y, base);
                const h = bottom - top;
                if (h < 1 && n === 0) return;
                ctx.font =
                  h >= 22
                    ? "600 12px system-ui, 'Segoe UI', sans-serif"
                    : "600 11px system-ui, 'Segoe UI', sans-serif";
                if (h >= 16) {
                  ctx.fillStyle = "rgba(255,255,255,0.92)";
                  ctx.textBaseline = "middle";
                  ctx.fillText(String(n), x, (top + bottom) / 2);
                } else {
                  ctx.fillStyle = "#e8edf4";
                  ctx.textBaseline = "bottom";
                  ctx.fillText(String(n), x, top - 4);
                }
              });
              ctx.restore();
            },
          },
        ],
      });
    }
  } catch (e) {
    console.error(e);
    alert("加载 Trend 失败: " + e.message);
  }
}

async function openCustomerEditDialog(customerId) {
  if (!dlgCustomer || !formCustomer) return;
  try {
    const c = await api(`/api/customers/${encodeURIComponent(customerId)}`);
    document.getElementById("customer-edit-id").value = c.id || "";
    const idname = document.getElementById("customer-edit-idname");
    if (idname) idname.textContent = `${c.id} — ${(c.name || "").trim() || c.id}`;
    document.getElementById("customer-edit-name").value = (c.name || "").trim();
    document.getElementById("customer-edit-email").value = c.email || "";
    document.getElementById("customer-edit-phone").value = c.phone || "";
    document.getElementById("customer-edit-address").value = c.address || "";
    const inactive = String(c.status || "active").toLowerCase() === "inactive";
    document.getElementById("customer-edit-inactive").checked = inactive;
    dlgCustomer.showModal();
  } catch (e) {
    alert("加载客户失败: " + e.message);
  }
}

formCustomer?.addEventListener("submit", async (e) => {
  e.preventDefault();
  const id = document.getElementById("customer-edit-id")?.value?.trim();
  if (!id) return;
  const name = document.getElementById("customer-edit-name")?.value?.trim() || "";
  const email = document.getElementById("customer-edit-email")?.value?.trim() || "";
  const phone = document.getElementById("customer-edit-phone")?.value?.trim() || "";
  if (!name) {
    alert("Customer Name 不能为空。");
    return;
  }
  if (email && !emailFormatRx.test(email)) {
    alert("Email 格式不正确，请使用 xxx@xxx.com。");
    return;
  }
  if (phone && !phoneFormatRx.test(phone)) {
    alert("Phone Number 格式不正确，仅支持 +000000000000 或纯数字。");
    return;
  }
  const inactive = document.getElementById("customer-edit-inactive")?.checked;
  const body = {
    name,
    email,
    phone,
    address: document.getElementById("customer-edit-address")?.value ?? "",
    status: inactive ? "inactive" : "active",
  };
  try {
    await api(`/api/customers/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(body) });
    await loadCustomers();
    await loadTasks();
    dlgCustomer?.close();
    if (invoiceViewMode === "customers") {
      renderCustomersList();
    }
    if (invoiceViewMode === "new-invoice") {
      renderNewInvoiceView();
    }
    const selId = document.getElementById("task-customer")?.value;
    fillTaskCustomerSelect(selId || "");
  } catch (err) {
    alert("保存失败: " + err.message);
  }
});

document.getElementById("customer-edit-cancel")?.addEventListener("click", () => dlgCustomer?.close());
document.getElementById("btn-invoices-back-payment")?.addEventListener("click", () => setInvoiceView("list"));
document.getElementById("btn-invoices-back-new")?.addEventListener("click", () => setInvoiceView("list"));

document.getElementById("btn-invoicing-go")?.addEventListener("click", () => {
  const selected = [...document.querySelectorAll(".new-inv-cb:checked")]
    .map((x) => tasksCache.find((t) => t.id === x.dataset.taskId))
    .filter(Boolean);
  if (selected.length === 0) return;
  const custIds = new Set(selected.map((t) => (t.customerId || "").trim()));
  if (custIds.size !== 1 || [...custIds][0] === "") return;
  openInvoiceDialogMulti(selected);
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
  const sync = document.getElementById("price-sync-pending");
  if (sync) sync.checked = false;
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
    const sync = document.getElementById("price-sync-pending")?.checked;
    const q = sync ? "?syncPendingTasks=1" : "";
    let res;
    if (id) {
      res = await api(`/api/prices/${encodeURIComponent(id)}${q}`, { method: "PUT", body: JSON.stringify(payload) });
    } else {
      res = await api(`/api/prices${q}`, { method: "POST", body: JSON.stringify(payload) });
    }
    if (typeof res.syncedPendingTasks === "number" && res.syncedPendingTasks > 0) {
      alert(`已同步 ${res.syncedPendingTasks} 个 Pending 任务的 Task / Price。`);
      await loadTasks();
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

// --- Expenses ---
let expensesCache = [];
let expenseCatalogCache = [];
let expenseVendorsCache = [];

async function loadExpenseCatalog() {
  try {
    expenseCatalogCache = asArray(await api("/api/expense-codes/catalog"));
  } catch (e) {
    console.error(e);
    expenseCatalogCache = [];
  }
}

async function loadExpenseVendors() {
  try {
    expenseVendorsCache = asArray(await api("/api/expense-vendors"));
  } catch (e) {
    console.error(e);
    expenseVendorsCache = [];
  }
}

async function loadExpenses() {
  try {
    await loadTasks();
    await loadExpenseCatalog();
    await loadExpenseVendors();
    expensesCache = asArray(await api("/api/expenses"));
    renderExpenses();
  } catch (e) {
    console.error(e);
    alert("加载 Expense 失败: " + e.message);
  }
}

function fillExpenseTaskSelect(selectedId) {
  const sel = document.getElementById("expense-task-id");
  if (!sel) return;
  sel.innerHTML = "";
  const sorted = [...tasksCache].sort((a, b) => String(a.id).localeCompare(String(b.id)));
  for (const t of sorted) {
    const o = document.createElement("option");
    o.value = t.id;
    o.textContent = (t.companyName || t.id).trim() || t.id;
    sel.appendChild(o);
  }
  if (selectedId) sel.value = selectedId;
}

function fillExpenseVendorSelect(selectedId) {
  const sel = document.getElementById("expense-vendor-id");
  if (!sel) return;
  sel.innerHTML = "";
  const ph = document.createElement("option");
  ph.value = "";
  ph.textContent = "— Optional —";
  sel.appendChild(ph);
  for (const v of expenseVendorsCache) {
    const o = document.createElement("option");
    o.value = v.id;
    o.textContent = (v.name || v.id).trim() || v.id;
    sel.appendChild(o);
  }
  const vid = selectedId != null ? String(selectedId).trim() : "";
  if (vid && !expenseVendorsCache.some((v) => String(v.id) === vid)) {
    const o = document.createElement("option");
    o.value = vid;
    o.textContent = `${vid}（vendor missing）`;
    sel.appendChild(o);
  }
  if (vid) sel.value = vid;
}

/** Expense account：仅 Settings 中已添加的 expense_codes */
function fillExpenseAccountSelect(selectedCode, allowOrphan) {
  const sel = document.getElementById("expense-account-code");
  if (!sel) return;
  sel.innerHTML = "";
  const ph = document.createElement("option");
  ph.value = "";
  ph.textContent = expenseCatalogCache.length
    ? "— 选择科目 —"
    : "— 请先在 Settings → Expense Code 添加科目 —";
  sel.appendChild(ph);
  for (const row of expenseCatalogCache) {
    const o = document.createElement("option");
    o.value = row.code;
    o.textContent = row.name ? `${row.code} — ${row.name}` : row.code;
    sel.appendChild(o);
  }
  const code = selectedCode != null ? String(selectedCode).trim() : "";
  if (allowOrphan && /^5\d{3}$/.test(code) && !expenseCatalogCache.some((r) => String(r.code) === code)) {
    const o = document.createElement("option");
    o.value = code;
    o.textContent = `${code}（未在目录，请先在 Settings 添加）`;
    sel.appendChild(o);
  }
  if (code) sel.value = code;
}

function renderExpenses() {
  const body = document.getElementById("expenses-body");
  if (!body) return;
  body.innerHTML = "";
  for (const ex of asArray(expensesCache)) {
    const tr = document.createElement("tr");
    const amt = fmtNum(ex.amount);
    const cur = ex.currency || "";
    const d = ex.expenseDate || "";
    tr.innerHTML = `
      <td>${escapeHtml(ex.taskName || "")}</td>
      <td>${escapeHtml(ex.vendorName || "—")}</td>
      <td>${escapeHtml(d || "—")}</td>
      <td>${escapeHtml(ex.accountCode || "—")}</td>
      <td>${escapeHtml(ex.description || "")}</td>
      <td>${escapeHtml(amt)} ${escapeHtml(cur)}</td>
      <td class="row-actions"><button type="button" class="ghost" data-act="edit">Edit</button></td>`;
    tr.querySelector('[data-act="edit"]').addEventListener("click", () => openExpenseDialog(ex));
    body.appendChild(tr);
  }
}

const dlgExpense = document.getElementById("dlg-expense");
const formExpense = document.getElementById("form-expense");

/** 从任务行跳转：切到 Expense 标签并打开新建支出，Task 已选当前行 */
async function openNewExpenseForTask(t) {
  if (!t || !t.id) return;
  document.querySelectorAll(".tab").forEach((b) => b.classList.toggle("active", b.dataset.tab === "expense"));
  document.querySelectorAll(".panel").forEach((p) => p.classList.toggle("active", p.id === "panel-expense"));
  await loadExpenses();
  await openExpenseDialog({ taskId: t.id });
}

async function openExpenseDialog(ex) {
  await loadExpenseCatalog();
  await loadExpenseVendors();
  const isEdit = !!(ex && ex.id);
  const title = document.getElementById("dlg-expense-title");
  if (title) title.textContent = isEdit ? "Edit expense" : "New expense";
  const hid = document.getElementById("expense-id");
  if (hid) hid.value = isEdit ? ex.id : "";
  fillExpenseTaskSelect((ex && ex.taskId) || "");
  fillExpenseVendorSelect((ex && ex.vendorId) || "");
  const accCode = ex && ex.accountCode != null ? String(ex.accountCode).trim() : "";
  fillExpenseAccountSelect(accCode, isEdit);
  const dateEl = document.getElementById("expense-date");
  if (dateEl) {
    dateEl.value =
      ex && ex.expenseDate && /^\d{4}-\d{2}-\d{2}$/.test(String(ex.expenseDate))
        ? String(ex.expenseDate)
        : todayLocalISO();
  }
  const desc = document.getElementById("expense-description");
  if (desc) desc.value = (ex && ex.description) || "";
  const amt = document.getElementById("expense-amount");
  if (amt) amt.value = ex != null && ex.amount != null && !Number.isNaN(ex.amount) ? String(ex.amount) : "";
  const cur = document.getElementById("expense-currency");
  if (cur) cur.value = (ex && ex.currency) || "CAD";
  if (dlgExpense) dlgExpense.showModal();
}

const btnNewExpense = document.getElementById("btn-new-expense");
if (btnNewExpense) btnNewExpense.addEventListener("click", () => openExpenseDialog(null).catch((e) => alert(e.message)));

const expenseCancel = document.getElementById("expense-cancel");
if (expenseCancel) expenseCancel.addEventListener("click", () => dlgExpense && dlgExpense.close());

if (formExpense) {
  formExpense.addEventListener("submit", async (e) => {
    e.preventDefault();
    const id = document.getElementById("expense-id")?.value?.trim() || "";
    const taskId = document.getElementById("expense-task-id")?.value?.trim() || "";
    const accountCode = document.getElementById("expense-account-code")?.value?.trim() || "";
    const expenseDate = document.getElementById("expense-date")?.value?.trim() || "";
    const vendorId = document.getElementById("expense-vendor-id")?.value?.trim() || "";
    const payload = {
      taskId,
      vendorId,
      accountCode,
      expenseDate,
      description: document.getElementById("expense-description")?.value?.trim() || "",
      amount: parseFloat(document.getElementById("expense-amount")?.value || "0"),
      currency: document.getElementById("expense-currency")?.value || "CAD",
    };
    if (!taskId) {
      alert("请选择 Task。");
      return;
    }
    if (!expenseDate) {
      alert("请选择 Date。");
      return;
    }
    if (!/^5\d{3}$/.test(accountCode)) {
      alert("请选择 Expense account（请先在 Settings → Expense Code 添加 5XXX 科目）。");
      return;
    }
    if (Number.isNaN(payload.amount) || payload.amount < 0) {
      alert("请输入有效金额。");
      return;
    }
    try {
      if (id) {
        await api(`/api/expenses/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(payload) });
      } else {
        await api("/api/expenses", { method: "POST", body: JSON.stringify(payload) });
      }
      dlgExpense.close();
      await loadExpenses();
    } catch (err) {
      alert("保存失败: " + err.message);
    }
  });
}

const dlgVendor = document.getElementById("dlg-vendor");
const formVendor = document.getElementById("form-vendor");

document.getElementById("btn-expense-new-vendor")?.addEventListener("click", () => {
  const n = document.getElementById("vendor-name");
  const em = document.getElementById("vendor-email");
  const ad = document.getElementById("vendor-address");
  const cu = document.getElementById("vendor-currency");
  if (n) n.value = "";
  if (em) em.value = "";
  if (ad) ad.value = "";
  if (cu) cu.value = "CAD";
  dlgVendor?.showModal();
});

document.getElementById("vendor-cancel")?.addEventListener("click", () => dlgVendor?.close());

if (formVendor) {
  formVendor.addEventListener("submit", async (e) => {
    e.preventDefault();
    const name = document.getElementById("vendor-name")?.value?.trim() || "";
    if (!name) {
      alert("请输入 Vendor name。");
      return;
    }
    const body = {
      name,
      currency: document.getElementById("vendor-currency")?.value || "CAD",
      email: document.getElementById("vendor-email")?.value?.trim() || "",
      address: document.getElementById("vendor-address")?.value?.trim() || "",
    };
    try {
      const created = await api("/api/expense-vendors", { method: "POST", body: JSON.stringify(body) });
      await loadExpenseVendors();
      fillExpenseVendorSelect(created && created.id ? created.id : "");
      dlgVendor?.close();
    } catch (err) {
      alert("保存失败: " + err.message);
    }
  });
}

// --- Exchange rates (Frankfurter, server cache) ---
let exchangeConvTimer = null;
let exchangeConvCurrencyList = [];
let exchangeConvLastFixed = "from";

async function ensureExchangeCurrencyList() {
  if (exchangeConvCurrencyList.length) return;
  exchangeConvCurrencyList = asArray(await api("/api/exchange-rates/currencies"));
  const from = document.getElementById("exchange-conv-from");
  const to = document.getElementById("exchange-conv-to");
  if (!from || !to) return;
  from.innerHTML = "";
  to.innerHTML = "";
  for (const row of exchangeConvCurrencyList) {
    const code = row.code || "";
    const name = row.name || code;
    const lab = `${code} — ${name}`;
    const o1 = document.createElement("option");
    o1.value = code;
    o1.textContent = lab;
    from.appendChild(o1);
    const o2 = document.createElement("option");
    o2.value = code;
    o2.textContent = lab;
    to.appendChild(o2);
  }
  if (from.querySelector('option[value="USD"]')) from.value = "USD";
  if (to.querySelector('option[value="EUR"]')) to.value = "EUR";
}

function initExchangeConverterDate() {
  const d = document.getElementById("exchange-conv-date");
  if (d && !d.value) d.value = todayLocalISO();
  const fa = document.getElementById("exchange-conv-from-amt");
  if (fa && String(fa.value).trim() === "") fa.value = "1";
}

async function runExchangeConverter(fixed) {
  const date = document.getElementById("exchange-conv-date")?.value?.trim();
  const from = document.getElementById("exchange-conv-from")?.value?.trim();
  const to = document.getElementById("exchange-conv-to")?.value?.trim();
  const rateEl = document.getElementById("exchange-conv-rate");
  const noteEl = document.getElementById("exchange-conv-note");
  if (!date) {
    if (rateEl) rateEl.textContent = "";
    if (noteEl) noteEl.textContent = "请选择换算日期。";
    return;
  }
  if (!from || !to) {
    if (rateEl) rateEl.textContent = "";
    return;
  }
  if (from === to) {
    if (rateEl) rateEl.textContent = "";
    if (noteEl) noteEl.textContent = "请选择两个不同的货币。";
    return;
  }
  const fromAmt = parseFloat(document.getElementById("exchange-conv-from-amt")?.value || "");
  const toAmt = parseFloat(document.getElementById("exchange-conv-to-amt")?.value || "");
  let amt = fixed === "to" ? toAmt : fromAmt;
  if (Number.isNaN(amt) || amt < 0) amt = 0;
  const params = new URLSearchParams({ date, from, to, amount: String(amt), fixed });
  try {
    const data = await api(`/api/exchange-rates/convert?${params.toString()}`);
    const fa = document.getElementById("exchange-conv-from-amt");
    const ta = document.getElementById("exchange-conv-to-amt");
    if (fa && data.amountFrom != null && !Number.isNaN(data.amountFrom)) fa.value = String(data.amountFrom);
    if (ta && data.amountTo != null && !Number.isNaN(data.amountTo)) ta.value = String(data.amountTo);
    if (rateEl) rateEl.textContent = (data.rateLabel || "") + (data.live ? " · 即时" : "");
    if (noteEl) noteEl.textContent = data.note || "";
  } catch (e) {
    if (rateEl) rateEl.textContent = "";
    if (noteEl) noteEl.textContent = e.message || "换算失败";
  }
}

function scheduleExchangeConv(fixed) {
  exchangeConvLastFixed = fixed;
  clearTimeout(exchangeConvTimer);
  exchangeConvTimer = setTimeout(() => runExchangeConverter(fixed).catch((err) => console.error(err)), 280);
}

async function loadExchangeTab() {
  await ensureExchangeCurrencyList();
  initExchangeConverterDate();
  await runExchangeConverter("from");
  await loadExchangeRates();
}

function fmtExchangeRate(n) {
  if (n == null || typeof n !== "number" || Number.isNaN(n)) return "—";
  const a = Math.abs(n);
  const opts =
    a >= 1
      ? { minimumFractionDigits: 4, maximumFractionDigits: 4 }
      : { minimumFractionDigits: 4, maximumFractionDigits: 6 };
  return n.toLocaleString("en-CA", opts);
}

function buildExchangeRatesURL() {
  const q = document.getElementById("exchange-q")?.value?.trim() || "";
  const day = document.getElementById("exchange-date")?.value?.trim() || "";
  const params = new URLSearchParams();
  if (q) params.set("q", q);
  if (day) params.set("date", day);
  const qs = params.toString();
  return { url: qs ? `/api/exchange-rates?${qs}` : "/api/exchange-rates" };
}

async function loadExchangeRates() {
  const sum = document.getElementById("exchange-summary");
  const thead = document.getElementById("exchange-thead");
  const tbody = document.getElementById("exchange-body");
  if (!thead || !tbody) return;
  const built = buildExchangeRatesURL();
  if (sum) sum.textContent = "加载中…";
  tbody.innerHTML = "";
  thead.innerHTML = "";
  try {
    const data = await api(built.url);
    const dates = asArray(data.dates);
    const rows = asArray(data.rows);
    const base = data.base || "";
    const dayRange =
      dates.length >= 2 ? `${dates[0]} … ${dates[dates.length - 1]}` : dates.length ? dates[0] : "—";
    if (sum) {
      if (rows.length === 0) {
        sum.textContent = `基准 ${base}；列日期 ${dayRange}（${dates.length} 个工作日）。未在 Settings → 汇率货币 中添加报价币种则表格为空；非当日列优先本地缓存，缺失则拉取 Frankfurter 并写入库。`;
      } else {
        sum.textContent = `基准 ${base}；列日期 ${dayRange}（${dates.length} 个工作日）；共 ${rows.length} 种货币（Settings → 汇率货币）。当日列为即时汇率且不写入库。`;
      }
    }
    const hr = document.createElement("tr");
    const thCur = document.createElement("th");
    thCur.textContent = "Currency";
    hr.appendChild(thCur);
    for (const d of dates) {
      const th = document.createElement("th");
      th.className = "num";
      th.textContent = d;
      hr.appendChild(th);
    }
    thead.appendChild(hr);
    for (const r of rows) {
      const tr = document.createElement("tr");
      const td0 = document.createElement("td");
      const name = escapeHtml(r.name || "");
      const code = escapeHtml(r.code || "");
      td0.innerHTML = `${name} <span class="exchange-code">(${code})</span>`;
      tr.appendChild(td0);
      const rateMap = r.rates && typeof r.rates === "object" ? r.rates : {};
      for (const d of dates) {
        const td = document.createElement("td");
        td.className = "num";
        td.textContent = fmtExchangeRate(rateMap[d]);
        tr.appendChild(td);
      }
      tbody.appendChild(tr);
    }
  } catch (e) {
    console.error(e);
    if (sum) sum.textContent = "加载失败。";
    alert("Exchange Rate 加载失败: " + e.message);
  }
}

document.getElementById("btn-exchange-load")?.addEventListener("click", () => loadExchangeRates());
document.getElementById("exchange-q")?.addEventListener("keydown", (ev) => {
  if (ev.key === "Enter") {
    ev.preventDefault();
    loadExchangeRates();
  }
});

document.getElementById("exchange-conv-date")?.addEventListener("change", () => scheduleExchangeConv(exchangeConvLastFixed));
document.getElementById("exchange-conv-from-amt")?.addEventListener("input", () => scheduleExchangeConv("from"));
document.getElementById("exchange-conv-to-amt")?.addEventListener("input", () => scheduleExchangeConv("to"));
document.getElementById("exchange-conv-from")?.addEventListener("change", () => scheduleExchangeConv("from"));
document.getElementById("exchange-conv-to")?.addEventListener("change", () => scheduleExchangeConv("from"));
document.getElementById("exchange-conv-swap")?.addEventListener("click", () => {
  const fs = document.getElementById("exchange-conv-from");
  const ts = document.getElementById("exchange-conv-to");
  if (!fs || !ts) return;
  const x = fs.value;
  fs.value = ts.value;
  ts.value = x;
  scheduleExchangeConv("from");
});

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
        <td>${escapeHtml(t.customerName || "")}</td>
        <td>${escapeHtml(t.companyName || "")}</td>
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
  const headers = ["No.", "Customer", "Task Name", "日期", "Task", "Price", "状态", "完成日期", "备注"];
  const lines = [headers.join(",")];
  for (const t of rows) {
    lines.push(
      [
        csvEscape(t.id),
        csvEscape(t.customerName),
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
