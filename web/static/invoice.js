function qs(name) {
  return new URLSearchParams(window.location.search).get(name);
}

function fmtMoney(v, currency) {
  const n = Number(v || 0);
  return `${currency} ${n.toLocaleString("en-CA", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function formatDisplayDate(s) {
  const t = String(s || "").trim();
  if (!t) return "";
  if (t.length >= 10 && /^\d{4}-\d{2}-\d{2}/.test(t)) {
    return t.slice(0, 4) + "/" + t.slice(5, 7) + "/" + t.slice(8, 10) + t.slice(10);
  }
  return t;
}

async function loadInvoice() {
  const id = qs("id");
  if (!id) {
    alert("缺少 invoice id");
    return;
  }
  const r = await fetch(`/api/invoices/${encodeURIComponent(id)}`, { credentials: "same-origin" });
  if (r.status === 401) {
    window.location.href = "/login.html";
    return;
  }
  if (!r.ok) {
    alert("加载发票失败");
    return;
  }
  const inv = await r.json();
  const c = inv.currency || "USD";

  document.getElementById("bill-name").textContent = inv.billToName || "";
  document.getElementById("bill-addr").textContent = inv.billToAddr || "";
  document.getElementById("ship-name").textContent = inv.shipToName || "";
  document.getElementById("ship-addr").textContent = inv.shipToAddr || "";
  document.getElementById("invoice-no").textContent = inv.invoiceNo || "";
  document.getElementById("invoice-date").textContent = formatDisplayDate(inv.invoiceDate || "");
  document.getElementById("terms").textContent = inv.terms || "";
  document.getElementById("due-date").textContent = formatDisplayDate(inv.dueDate || "");

  const body = document.getElementById("items-body");
  body.innerHTML = "";
  (inv.items || []).forEach((it) => {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${formatDisplayDate(inv.invoiceDate || "")}</td>
      <td><div>${it.description || ""}</div><div>${it.detail || ""}</div></td>
      <td>${it.taxLabel || ""}</td>
      <td class="num">${Number(it.qty || 0)}</td>
      <td class="num">${fmtMoney(it.rate || 0, c)}</td>
      <td class="num">${fmtMoney(it.amount || 0, c)}</td>`;
    body.appendChild(tr);
  });

  document.getElementById("subtotal").textContent = fmtMoney(inv.subtotal, c);
  document.getElementById("tax-label").textContent = `GST @ ${Number(inv.taxRate || 0)}%`;
  document.getElementById("tax-amount").textContent = fmtMoney(inv.taxAmount, c);
  document.getElementById("total").textContent = fmtMoney(inv.total, c);
  document.getElementById("balance").textContent = fmtMoney(inv.balanceDue, c);
}

document.getElementById("btn-print").addEventListener("click", () => window.print());
document.getElementById("btn-back").addEventListener("click", () => (window.location.href = "/"));
loadInvoice();
