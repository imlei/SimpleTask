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
  const text = await r.text();
  if (!r.ok) throw new Error(text || r.statusText);
  return text ? JSON.parse(text) : null;
}

let logoDataUrl = "";

function showLogoPreview(dataUrl) {
  const box = document.getElementById("set-logo-preview");
  const clr = document.getElementById("set-logo-clear");
  if (!dataUrl) {
    box.hidden = true;
    box.innerHTML = "";
    clr.hidden = true;
    return;
  }
  box.hidden = false;
  clr.hidden = false;
  box.innerHTML = `<img src="${dataUrl}" alt="Logo" />`;
}

document.getElementById("set-logo-file")?.addEventListener("change", (e) => {
  const f = e.target.files?.[0];
  if (!f) return;
  const reader = new FileReader();
  reader.onload = () => {
    logoDataUrl = String(reader.result || "");
    if (logoDataUrl.length > 500000) {
      alert("图片过大，请压缩后小于约 350KB。");
      logoDataUrl = "";
      e.target.value = "";
      showLogoPreview("");
      return;
    }
    showLogoPreview(logoDataUrl);
  };
  reader.readAsDataURL(f);
});

document.getElementById("set-logo-clear")?.addEventListener("click", () => {
  logoDataUrl = "";
  const input = document.getElementById("set-logo-file");
  if (input) input.value = "";
  showLogoPreview("");
});

async function loadSettings() {
  const s = await api("/api/settings");
  document.getElementById("set-company").value = s.companyName || "";
  document.getElementById("set-baseurl").value = s.baseUrl || "";
  document.getElementById("set-smtp-host").value = s.smtpHost || "";
  document.getElementById("set-smtp-port").value = s.smtpPort || 587;
  document.getElementById("set-smtp-user").value = s.smtpUser || "";
  document.getElementById("set-smtp-pass").value = "";
  document.getElementById("set-smtp-pass").placeholder = s.smtpPassSet ? "已设置，留空不修改" : "未设置";
  document.getElementById("set-smtp-from").value = s.smtpFrom || "";
  document.getElementById("set-smtp-starttls").checked = s.smtpStartTls !== false;
  document.getElementById("set-smtp-tls").checked = !!s.smtpImplicitTls;
  const hint = document.getElementById("set-env-hint");
  if (hint) {
    const eh = s.envSmtpHost || "(未设置)";
    const eb = s.envBaseUrl || "(未设置)";
    hint.textContent = `当前服务器环境：SMTP_HOST=${eh}，BASE_URL=${eb}`;
  }
  logoDataUrl = s.logoDataUrl || "";
  showLogoPreview(logoDataUrl);
  await loadBankAccounts();
}

/** ABA routing 9 位校验（美国） */
function abaRoutingChecksumOk(d9) {
  if (!d9 || d9.length !== 9) return false;
  const a = d9.split("").map((c) => parseInt(c, 10));
  if (a.some((x) => Number.isNaN(x))) return false;
  const sum = 3 * (a[0] + a[3] + a[6]) + 7 * (a[1] + a[4] + a[7]) + 1 * (a[2] + a[5] + a[8]);
  return sum % 10 === 0;
}

function checkAbaRoutingInput(msgId, inputId) {
  const msg = document.getElementById(msgId);
  const inp = document.getElementById(inputId);
  if (!msg || !inp) return;
  const d = inp.value.replace(/\D/g, "");
  msg.textContent = "";
  if (d.length === 0) {
    msg.hidden = true;
    return;
  }
  if (d.length !== 9) {
    msg.hidden = false;
    msg.textContent = "ABA Routing 应为 9 位数字。";
    return;
  }
  if (!abaRoutingChecksumOk(d)) {
    msg.hidden = false;
    msg.textContent = "校验位与 ABA 算法不符，请核对支票或银行资料。";
    return;
  }
  msg.hidden = true;
}

function updateBankMicrCountryUI() {
  const sel = document.getElementById("bank-micr-country");
  if (!sel) return;
  const isUS = sel.value === "US";
  const ca = document.getElementById("bank-group-ca");
  const us = document.getElementById("bank-group-us");
  if (ca) ca.hidden = isUS;
  if (us) us.hidden = !isUS;
}

document.getElementById("bank-micr-country")?.addEventListener("change", updateBankMicrCountryUI);
document.getElementById("bank-routing")?.addEventListener("blur", () => checkAbaRoutingInput("bank-routing-aba-msg", "bank-routing"));

document.getElementById("btn-save-settings")?.addEventListener("click", async () => {
  const msg = document.getElementById("save-msg");
  msg.textContent = "";
  const body = {
    companyName: document.getElementById("set-company").value.trim(),
    baseUrl: document.getElementById("set-baseurl").value.trim(),
    logoDataUrl: logoDataUrl,
    smtpHost: document.getElementById("set-smtp-host").value.trim(),
    smtpPort: parseInt(document.getElementById("set-smtp-port").value, 10) || 587,
    smtpUser: document.getElementById("set-smtp-user").value.trim(),
    smtpPass: document.getElementById("set-smtp-pass").value,
    smtpFrom: document.getElementById("set-smtp-from").value.trim(),
    smtpStartTls: document.getElementById("set-smtp-starttls").checked,
    smtpImplicitTls: document.getElementById("set-smtp-tls").checked,
  };
  try {
    await api("/api/settings", { method: "PUT", body: JSON.stringify(body) });
    msg.textContent = "已保存。";
    await loadSettings();
  } catch (e) {
    alert("保存失败: " + e.message);
  }
});

async function loadBankAccounts() {
  const box = document.getElementById("bank-list");
  if (!box) return;
  box.innerHTML = "加载中…";
  let data;
  try {
    data = await api("/api/bank-accounts");
  } catch (e) {
    box.innerHTML = `<p class="hint">加载银行账户失败：${e.message}</p>`;
    return;
  }
  const items = data.items || [];
  const def = data.defaultId || "";
  if (items.length === 0) {
    box.innerHTML = `<p class="hint">尚未添加银行账户。请在下方表单添加，并可设置默认账户。</p>`;
    return;
  }
  box.innerHTML = items
    .map((b) => {
      const isDef = b.id === def;
      const badge = isDef ? `<strong>（默认）</strong>` : "";
      const safe = (s) => String(s || "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
      return `
        <div style="display:flex; gap:8px; align-items:center; justify-content:space-between; padding:8px 0; border-bottom:1px dashed #cbd5e1;">
          <div>
            <div><strong>${safe(b.label || b.id)}</strong> ${badge}</div>
            <div class="hint">${safe((b.micrCountry || "CA").toUpperCase())} · Cheque # ${safe(b.bankChequeNumber || "")} · Currency ${safe((b.defaultChequeCurrency || "CAD").toUpperCase())}</div>
          </div>
          <div style="display:flex; gap:6px; flex-wrap:wrap;">
            ${isDef ? "" : `<button type="button" class="ghost small" data-act="default" data-id="${safe(b.id)}">设为默认</button>`}
            <button type="button" class="ghost small" data-act="edit" data-id="${safe(b.id)}">编辑</button>
            <button type="button" class="ghost small" data-act="del" data-id="${safe(b.id)}">删除</button>
          </div>
        </div>
      `;
    })
    .join("");
  box.querySelectorAll("button[data-act]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const act = btn.dataset.act;
      const id = btn.dataset.id;
      if (act === "default") {
        await api(`/api/bank-accounts/${encodeURIComponent(id)}/default`, { method: "POST", body: "{}" });
        await loadBankAccounts();
        return;
      }
      if (act === "edit") {
        const it = items.find((x) => x.id === id);
        if (!it) return;
        fillBankForm(it);
        return;
      }
      if (act === "del") {
        if (!confirm("确定删除该银行账户？")) return;
        await api(`/api/bank-accounts/${encodeURIComponent(id)}`, { method: "DELETE" });
        await loadBankAccounts();
        return;
      }
    });
  });
}

function fillBankForm(b) {
  document.getElementById("bank-id").value = b.id || "";
  document.getElementById("bank-label").value = b.label || "";
  document.getElementById("bank-micr-country").value = (b.micrCountry || "CA").toUpperCase() === "US" ? "US" : "CA";
  document.getElementById("bank-institution").value = b.bankInstitution || "";
  document.getElementById("bank-transit").value = b.bankTransit || "";
  document.getElementById("bank-routing").value = b.bankRoutingAba || "";
  document.getElementById("bank-account").value = b.bankAccount || "";
  document.getElementById("bank-cheque").value = b.bankChequeNumber || "";
  document.getElementById("bank-micr-override").value = b.micrLineOverride || "";
  const cur = (b.defaultChequeCurrency || "CAD").toUpperCase();
  const sel = document.getElementById("bank-default-cheque-currency");
  if (sel) {
    sel.querySelectorAll("option[data-custom]").forEach((o) => o.remove());
    const allowed = ["CAD", "USD", "CNY", "EUR"];
    if (allowed.includes(cur)) {
      sel.value = cur;
    } else if (cur) {
      const opt = document.createElement("option");
      opt.value = cur;
      opt.textContent = `${cur}（自定义）`;
      opt.setAttribute("data-custom", "1");
      sel.appendChild(opt);
      sel.value = cur;
    } else {
      sel.value = "CAD";
    }
  }
  updateBankMicrCountryUI();
  checkAbaRoutingInput("bank-routing-aba-msg", "bank-routing");
}

function clearBankForm() {
  fillBankForm({
    id: "",
    label: "",
    micrCountry: "CA",
    bankInstitution: "",
    bankTransit: "",
    bankRoutingAba: "",
    bankAccount: "",
    bankChequeNumber: "000001",
    micrLineOverride: "",
    defaultChequeCurrency: "CAD",
  });
}

document.getElementById("btn-bank-clear")?.addEventListener("click", clearBankForm);
document.getElementById("btn-bank-save")?.addEventListener("click", async () => {
  const id = document.getElementById("bank-id").value.trim();
  const body = {
    label: document.getElementById("bank-label").value.trim(),
    micrCountry: document.getElementById("bank-micr-country").value.trim() || "CA",
    bankInstitution: document.getElementById("bank-institution").value.trim(),
    bankTransit: document.getElementById("bank-transit").value.trim(),
    bankRoutingAba: document.getElementById("bank-routing").value.trim(),
    bankAccount: document.getElementById("bank-account").value.trim(),
    bankChequeNumber: document.getElementById("bank-cheque").value.trim(),
    micrLineOverride: document.getElementById("bank-micr-override").value.trim(),
    defaultChequeCurrency: document.getElementById("bank-default-cheque-currency").value.trim() || "CAD",
  };
  try {
    if (id) {
      await api(`/api/bank-accounts/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(body) });
    } else {
      await api(`/api/bank-accounts`, { method: "POST", body: JSON.stringify(body) });
    }
    clearBankForm();
    await loadBankAccounts();
  } catch (e) {
    alert("保存失败: " + e.message);
  }
});

document.getElementById("btn-save-password")?.addEventListener("click", async () => {
  const oldPassword = document.getElementById("pwd-old").value;
  const newPassword = document.getElementById("pwd-new").value;
  const new2 = document.getElementById("pwd-new2").value;
  const msg = document.getElementById("pwd-msg");
  msg.hidden = true;
  if (newPassword !== new2) {
    msg.textContent = "两次新密码不一致";
    msg.hidden = false;
    return;
  }
  try {
    await api("/api/auth/password", {
      method: "POST",
      body: JSON.stringify({ oldPassword, newPassword }),
    });
    document.getElementById("pwd-old").value = "";
    document.getElementById("pwd-new").value = "";
    document.getElementById("pwd-new2").value = "";
    msg.textContent = "密码已更新，请牢记新密码。";
    msg.hidden = false;
    msg.style.color = "var(--muted, #8b9cb3)";
  } catch (e) {
    msg.textContent = e.message || "失败";
    msg.hidden = false;
  }
});

(async function init() {
  try {
    const me = await fetch("/api/me", { credentials: "same-origin" }).then((r) => r.json());
    const btn = document.getElementById("btn-logout");
    if (btn) {
      btn.hidden = !me.authEnabled;
      btn.addEventListener("click", async () => {
        await fetch("/api/logout", { method: "POST", credentials: "same-origin" });
        window.location.href = "/login.html";
      });
    }
    if (me.authEnabled && !me.authenticated) {
      window.location.href = "/login.html";
      return;
    }
    await loadSettings();
    clearBankForm();
  } catch {
    window.location.href = "/login.html";
  }
})();
