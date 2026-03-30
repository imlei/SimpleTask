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
}

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
  } catch {
    window.location.href = "/login.html";
  }
})();
