function setRaiseAmount(amt) {
    const input = document.getElementById("raise-amount-input");
    if (input) {
        input.value = Math.max(0, Math.floor(amt));
    }
}

// Ticks every [data-countdown-until] element down once a second, client-side.
// The server only pushes a fresh render when its own timer actually fires
// (join countdown, next-hand intermission), so between those pushes this is
// what makes the displayed number actually count down instead of sitting
// frozen at whatever value was last rendered.
setInterval(function() {
    document.querySelectorAll("[data-countdown-until]").forEach(function(el) {
        const until = parseInt(el.getAttribute("data-countdown-until"), 10);
        if (!until) {
            return;
        }
        const valueEl = el.querySelector("[data-countdown-value]");
        if (!valueEl) {
            return;
        }
        const remaining = Math.max(0, Math.ceil((until - Date.now()) / 1000));
        valueEl.textContent = remaining;
    });
}, 1000);

function togglePasswordVisibility(inputId, btn) {
    const input = document.getElementById(inputId);
    if (!input) return;
    const showing = input.type === "text";
    input.type = showing ? "password" : "text";
    if (btn) {
        btn.textContent = showing ? "Show" : "Hide";
        btn.setAttribute("aria-label", showing ? "Show password" : "Hide password");
    }
}

function copyRoomCode(code) {
    if (navigator.clipboard) {
        navigator.clipboard.writeText(code).then(() => {
            const btnText = document.getElementById("copy-btn-text");
            if (btnText) {
                btnText.textContent = "Copied!";
                setTimeout(() => {
                    btnText.textContent = "Copy";
                }, 2000);
            }
        }).catch(() => {
            prompt("Copy this 5-digit room code:", code);
        });
    } else {
        prompt("Copy this 5-digit room code:", code);
    }
}

// Replaces htmx's default hx-confirm (the browser's ugly native confirm())
// with a themed modal. Any element with hx-confirm keeps working exactly as
// before - only the presentation changes. Add data-confirm-danger="true" on
// an element to render its Confirm button as a destructive/red action.
document.addEventListener("htmx:confirm", function(evt) {
    if (!evt.detail.question) {
        return;
    }
    evt.preventDefault();

    const overlay = document.getElementById("confirm-modal-overlay");
    const msgEl = document.getElementById("confirm-modal-message");
    const acceptBtn = document.getElementById("confirm-modal-accept");
    const cancelBtn = document.getElementById("confirm-modal-cancel");
    if (!overlay || !msgEl || !acceptBtn || !cancelBtn) {
        evt.detail.issueRequest(true);
        return;
    }

    msgEl.textContent = evt.detail.question;
    const isDanger = evt.detail.elt && evt.detail.elt.closest("[data-confirm-danger]");
    acceptBtn.className = isDanger ? "btn-danger" : "btn-gold";
    overlay.classList.remove("hidden");

    function cleanup() {
        overlay.classList.add("hidden");
        acceptBtn.removeEventListener("click", onAccept);
        cancelBtn.removeEventListener("click", onCancel);
        overlay.removeEventListener("click", onOverlayClick);
        document.removeEventListener("keydown", onKeydown);
    }
    function onAccept() {
        cleanup();
        evt.detail.issueRequest(true);
    }
    function onCancel() {
        cleanup();
    }
    function onOverlayClick(e) {
        if (e.target === overlay) onCancel();
    }
    function onKeydown(e) {
        if (e.key === "Escape") onCancel();
    }

    acceptBtn.addEventListener("click", onAccept);
    cancelBtn.addEventListener("click", onCancel);
    overlay.addEventListener("click", onOverlayClick);
    document.addEventListener("keydown", onKeydown);
});

document.addEventListener("htmx:afterRequest", function(evt) {
    if (evt.detail.successful && evt.detail.xhr.status === 200) {
        const errorBoxes = document.querySelectorAll("#auth-error, #code-error");
        errorBoxes.forEach(el => {
            if (el && !el.hasChildNodes()) {
                el.innerHTML = "";
            }
        });
    }
});

document.addEventListener("htmx:responseError", function(evt) {
    console.error("HTMX request error:", evt.detail);
});
