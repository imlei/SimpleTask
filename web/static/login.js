(async function boot() {
  try {
    const pub = await fetch("/api/settings/public").then((r) => r.json());
    const lt = document.getElementById("login-title");
    if (pub.companyName && lt) {
      lt.textContent = pub.companyName;
      document.title = "登录 — " + pub.companyName;
    }
    if (pub.logoDataUrl) {
      const box = document.getElementById("login-brand-logo");
      if (box) {
        const img = document.createElement("img");
        img.src = pub.logoDataUrl;
        img.alt = "";
        box.appendChild(img);
        box.hidden = false;
      }
    }
  } catch {
    /* ignore */
  }
  try {
    const me = await fetch("/api/me", { credentials: "same-origin" }).then((r) => r.json());
    if (!me.authEnabled) {
      window.location.replace("/");
      return;
    }
    if (me.needsSetup) {
      window.location.replace("/setup.html");
      return;
    }
    if (me.authenticated) {
      redirectAfterLogin(me.role);
      return;
    }
  } catch {
    // 继续显示登录页
  }

  const form = document.getElementById("form-login");
  const errEl = document.getElementById("login-err");

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    errEl.hidden = true;
    const username = document.getElementById("login-user").value.trim();
    const password = document.getElementById("login-pass").value;
    try {
      const r = await fetch("/api/login", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: JSON.stringify({ username, password }),
      });
      let data = {};
      try { data = await r.json(); } catch { /* ignore */ }
      if (!r.ok) {
        errEl.textContent = data.error || "登录失败";
        errEl.hidden = false;
        return;
      }
      redirectAfterLogin(data.role);
    } catch (err) {
      errEl.textContent = err.message || "网络错误";
      errEl.hidden = false;
    }
  });

  const regForm = document.getElementById("form-reg");
  const regErr = document.getElementById("reg-err");
  const regOk = document.getElementById("reg-ok");

  regForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    regErr.hidden = true;
    regOk.hidden = true;
    const username = document.getElementById("reg-user").value.trim();
    const password = document.getElementById("reg-pass").value;
    const password2 = document.getElementById("reg-pass2").value;
    if (password !== password2) {
      regErr.textContent = "两次输入的密码不一致";
      regErr.hidden = false;
      return;
    }
    try {
      const r = await fetch("/api/register", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: JSON.stringify({ username, password }),
      });
      let data = {};
      try { data = await r.json(); } catch { /* ignore */ }
      if (!r.ok) {
        regErr.textContent = data.error || "注册失败";
        regErr.hidden = false;
        return;
      }
      regOk.textContent = "注册成功，正在跳转…";
      regOk.hidden = false;
      setTimeout(() => redirectAfterLogin("user1"), 800);
    } catch (err) {
      regErr.textContent = err.message || "网络错误";
      regErr.hidden = false;
    }
  });
})();

function redirectAfterLogin(role) {
  if (role === "user1") {
    window.location.href = "/payroll/dashboard.html";
  } else {
    window.location.href = "/";
  }
}

function switchTab(tab) {
  const panels = { login: "panel-login", reg: "panel-reg" };
  const tabs = { login: "tab-login", reg: "tab-reg" };
  for (const k of Object.keys(panels)) {
    document.getElementById(panels[k]).hidden = (k !== tab);
    document.getElementById(tabs[k]).classList.toggle("active", k === tab);
  }
}
