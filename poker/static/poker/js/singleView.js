/* --------------------------------------------------------------------------------------------------
Hole-card private view - adapted for Django API
---------------------------------------------------------------------------------------------------*/
const singleViewEl = document.getElementById("single");
const cardSlots = document.querySelectorAll("img");
const nameBadge = document.querySelector("h3");
const chipsEl = document.querySelector(".total");
const betEl = document.querySelector(".bet");
const potEl = document.querySelector("#pot");
const notificationsEl = document.querySelector("#singleview-notifications");
const onlineOnlyElements = [betEl, potEl, notificationsEl];
const tableIdInput = document.querySelector("#table-id");
const paramInput = document.querySelector("#param-data");
const urlParams = new URLSearchParams(window.location.search);
const params = urlParams.get("params") ? urlParams.get("params").split("-") : (paramInput ? paramInput.value.split("-") : []);
const tableId = urlParams.get("table_id") || (tableIdInput ? tableIdInput.value : "");
const seatIndexParam = params[4] ? parseInt(params[4], 10) : null;
const API_BASE = "/api";
const REFRESH_INTERVAL = 2500;
let lastVersion = 0;
let pollTimeoutId = null;
let isPolling = false;

function init() {
    document.addEventListener("touchstart", function () {}, false);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    applyParams();
    pollState();
}

function applyParams() {
    const card1 = params[0];
    const card2 = params[1];
    const playerName = params[2];
    const chipsVal = parseInt(params[3], 10);
    setCards(card1, card2);
    if (nameBadge) nameBadge.textContent = playerName || "Player";
    if (chipsEl) chipsEl.textContent = chipsVal || 2000;
}

function setCards(card1, card2, folded) {
    if (card1 && cardSlots[0]) {
        cardSlots[0].src = `/static/poker/cards/${card1}.svg`;
    }
    if (card2 && cardSlots[1]) {
        cardSlots[1].src = `/static/poker/cards/${card2}.svg`;
    }
    if (folded !== undefined) {
        singleViewEl.classList.toggle("folded", folded);
    }
}

function setChips(amount, roundBet, pot) {
    if (typeof amount === "number" && chipsEl) chipsEl.textContent = amount;
    if (typeof roundBet === "number" && betEl) betEl.textContent = roundBet;
    if (typeof pot === "number" && potEl) potEl.textContent = pot;
}

function setOnlineElementsVisible(isOnline) {
    onlineOnlyElements.forEach((el) => {
        if (!el) return;
        el.classList.toggle("hidden", !isOnline);
    });
}

function renderNotifications(notifications) {
    if (!notificationsEl) return;
    notificationsEl.innerHTML = "";
    for (const msg of notifications) {
        const item = document.createElement("div");
        item.textContent = msg;
        notificationsEl.appendChild(item);
    }
}

async function pollState() {
    if (isPolling || document.visibilityState !== "visible") return;
    isPolling = true;
    try {
        const url = `${API_BASE}/state/?table_id=${encodeURIComponent(tableId)}&since_version=${lastVersion}`;
        const res = await fetch(url);
        if (res.status === 204) {
            setOnlineElementsVisible(true);
            return;
        }
        if (res.ok) {
            const payload = await res.json();
            lastVersion = payload.version;
            applyRemoteState(payload);
            setOnlineElementsVisible(true);
        } else {
            setOnlineElementsVisible(false);
        }
    } catch (e) {
        console.warn("state fetch failed", e);
        setOnlineElementsVisible(false);
    } finally {
        isPolling = false;
        schedulePoll();
    }
}

function schedulePoll() {
    if (document.visibilityState !== "visible") {
        pollTimeoutId = null;
        return;
    }
    pollTimeoutId = setTimeout(pollState, REFRESH_INTERVAL);
}

function handleVisibilityChange() {
    if (pollTimeoutId !== null) {
        clearTimeout(pollTimeoutId);
        pollTimeoutId = null;
    }
    if (document.visibilityState !== "visible") return;
    if (!isPolling) pollState();
}

function applyRemoteState(payload) {
    if (!payload || !payload.state || !Array.isArray(payload.state.players)) return;
    const player = payload.state.players.find((p) => p.seat_index === seatIndexParam);
    if (!player) return;
    if (nameBadge) nameBadge.textContent = player.name;
    const pot = payload.state.pot || 0;
    const cards = player.cards || [];
    setCards(cards[0], cards[1], player.folded);
    setChips(player.chips, player.round_bet, pot);
    renderNotifications(payload.notifications || []);
}

init();
