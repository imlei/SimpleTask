(async function boot() {
  try {
    const me = await fetch("/api/me", { credentials: "same-origin" }).then((r) => r.json());
    if (!me.authEnabled) {
      window.location.replace("/");
      return;
    }
    if (me.authenticated) {
      window.location.replace("/");
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
      try {
        data = await r.json();
      } catch {
        /* ignore */
      }
      if (!r.ok) {
        errEl.textContent = data.error || "登录失败";
        errEl.hidden = false;
        return;
      }
      window.location.href = "/";
    } catch (err) {
      errEl.textContent = err.message || "网络错误";
      errEl.hidden = false;
    }
  });
})();
