/* --------------------------------------------------------------------------------------------------
Django-adapted client for the Poker table
- Communicates with Django REST API for game logic
- Handles UI rendering, QR codes, notifications, animations
---------------------------------------------------------------------------------------------------*/
const startButton = document.querySelector("#start-button");
const foldButton = document.querySelector("#fold-button");
const actionButton = document.querySelector("#action-button");
const amountSlider = document.querySelector("#amount-slider");
const sliderOutput = document.querySelector("output");
const notification = document.querySelector("#notification");
const infoPot = document.querySelector("#pot");
const rotateIcons = document.querySelectorAll(".seat .rotate");
const nameBadges = document.querySelectorAll("h3");
const closeButtons = document.querySelectorAll(".close");
const tableIdInput = document.querySelector("#table-id");

let tableId = tableIdInput ? tableIdInput.value : "";
let gameState = null;
let players = [];
let previousVersion = 0;
let isPolling = false;
let pollTimeoutId = null;
const POLL_INTERVAL = 1500;
const MAX_ITEMS = 8;
const notifArr = [];
const pendingNotif = [];
let isNotifProcessing = false;
let NOTIF_INTERVAL = 750;
let ACTION_LABEL_DURATION = 3000;

const API_BASE = "/api";

function shuffle(a) {
    let i = a.length;
    while (i) {
        const j = Math.floor(Math.random() * i);
        const t = a[--i];
        a[i] = a[j];
        a[j] = t;
    }
    return a;
}

function createPlayers() {
    let botIndex = 1;
    const seats = document.querySelectorAll(".seat");
    for (const seat of seats) {
        const nameEl = seat.querySelector("h3");
        if (seat.classList.contains("hidden")) continue;
        if (nameEl.textContent.trim() === "") {
            nameEl.textContent = `Bot ${botIndex++}`;
            seat.classList.add("bot");
        }
    }
    const activeSeats = document.querySelectorAll(".seat:not(.hidden)");
    const playerList = [];
    for (const seat of activeSeats) {
        playerList.push(seat.querySelector("h3").textContent);
    }
    return playerList;
}

function applyState(state) {
    if (!state) return;
    gameState = state;
    players = state.players || [];

    // Update pot
    if (infoPot) infoPot.textContent = state.pot || 0;

    // Update community cards
    const communityCards = state.community_cards || [];
    const slots = document.querySelectorAll("#community-cards .cardslot");
    slots.forEach((slot, i) => {
        if (i < communityCards.length) {
            slot.innerHTML = `<img src="/static/poker/cards/${communityCards[i]}.svg">`;
        } else if (!slot.querySelector("img")) {
            slot.innerHTML = "";
        }
    });

    // Update seats
    players.forEach((p, index) => {
        const seat = document.querySelectorAll(".seat:not(.hidden)")[index];
        if (!seat) return;

        const nameEl = seat.querySelector("h3");
        if (nameEl) nameEl.textContent = p.name;

        const totalEl = seat.querySelector(".chips .total");
        if (totalEl) totalEl.textContent = p.chips;

        const betEl = seat.querySelector(".chips .bet");
        if (betEl) betEl.textContent = p.round_bet || 0;

        const cards = seat.querySelectorAll(".card");
        if (cards.length >= 2) {
            if (p.cards && p.cards[0] !== "1B") {
                cards[0].src = `/static/poker/cards/${p.cards[0]}.svg`;
                cards[1].src = `/static/poker/cards/${p.cards[1]}.svg`;
            } else {
                cards[0].src = "/static/poker/cards/1B.svg";
                cards[1].src = "/static/poker/cards/1B.svg";
            }
        }

        // Role visibility
        const dealerIcon = seat.querySelector(".dealer");
        if (dealerIcon) dealerIcon.classList.toggle("hidden", !p.dealer);
        const sbIcon = seat.querySelector(".small-blind");
        if (sbIcon) sbIcon.classList.toggle("hidden", !p.small_blind);
        const bbIcon = seat.querySelector(".big-blind");
        if (bbIcon) bbIcon.classList.toggle("hidden", !p.big_blind);

        // Folded/all-in state
        seat.classList.toggle("folded", p.folded);
        seat.classList.toggle("allin", p.all_in);

        // Win probability
        const wpEl = seat.querySelector(".win-probality");
        if (wpEl) {
            if (p.win_probability != null && state.spectator_mode) {
                wpEl.textContent = `${Math.round(p.win_probability)}%`;
                wpEl.classList.remove("hidden");
            } else {
                wpEl.classList.add("hidden");
            }
        }
    });
}

function applyNotifications(notifications) {
    if (!notifications) return;
    for (const msg of notifications) {
        notifArr.unshift(msg);
        if (notifArr.length > MAX_ITEMS) notifArr.pop();
        const span = document.createElement("span");
        span.textContent = msg;
        notification.prepend(span);
        while (notification.childElementCount > MAX_ITEMS) {
            notification.removeChild(notification.lastChild);
        }
    }
}

async function startGame(event) {
    const playerNames = createPlayers();
    if (playerNames.length < 2) {
        enqueueNotification("Not enough players");
        return;
    }
    startButton.classList.add("hidden");
    for (const ri of rotateIcons) ri.classList.add("hidden");
    for (const cb of closeButtons) cb.classList.add("hidden");
    for (const nb of nameBadges) nb.contentEditable = "false";

    if (!tableId) {
        tableId = Math.random().toString(36).slice(2, 8);
        if (tableIdInput) tableIdInput.value = tableId;
    }

    try {
        const resp = await fetch(`${API_BASE}/start/`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ table_id: tableId, players: playerNames }),
        });
        const data = await resp.json();
        if (data.ok) {
            tableId = data.table_id;
            previousVersion = data.state ? (data.state.total_hands || 0) : 0;
            applyState(data.state);
            const url = new URL(window.location);
            url.searchParams.set("table_id", tableId);
            window.history.replaceState(null, "", url.toString());
        }
    } catch (e) {
        console.error("start game failed", e);
        startButton.classList.remove("hidden");
    }
}

async function pollState() {
    if (isPolling) return;
    if (!tableId) return;
    isPolling = true;
    try {
        const url = `${API_BASE}/state/?table_id=${encodeURIComponent(tableId)}&since_version=${previousVersion}`;
        const resp = await fetch(url);
        if (resp.status === 204) return;
        if (resp.ok) {
            const data = await resp.json();
            previousVersion = data.version;
            applyState(data.state);
            applyNotifications(data.notifications);
            updateUIForPhase(data.state);
            startButton.classList.toggle("hidden", data.state.game_started);
        }
    } catch (e) {
        console.warn("poll failed", e);
    } finally {
        isPolling = false;
        schedulePoll();
    }
}

function schedulePoll() {
    if (pollTimeoutId) clearTimeout(pollTimeoutId);
    pollTimeoutId = setTimeout(pollState, POLL_INTERVAL);
}

function updateUIForPhase(state) {
    if (!state || !state.game_started || state.game_finished) {
        foldButton.classList.add("hidden");
        actionButton.classList.add("hidden");
        amountSlider.classList.add("hidden");
        sliderOutput.classList.add("hidden");
        return;
    }
    const activePlayers = (state.players || []).filter(p => !p.folded && !p.all_in);
    const isHumanTurn = activePlayers.some(p => !p.is_bot);
    if (isHumanTurn) {
        const humanPlayer = activePlayers.find(p => !p.is_bot);
        if (humanPlayer) {
            const needToCall = state.current_bet - humanPlayer.round_bet;
            foldButton.classList.remove("hidden");
            actionButton.classList.remove("hidden");
            amountSlider.classList.remove("hidden");
            sliderOutput.classList.remove("hidden");

            const minBet = Math.max(0, needToCall);
            amountSlider.min = 0;
            amountSlider.max = humanPlayer.chips;
            amountSlider.step = 10;
            amountSlider.value = minBet;
            sliderOutput.value = minBet;
            updateActionButton(needToCall, humanPlayer);
        }
    } else {
        foldButton.classList.add("hidden");
        actionButton.classList.add("hidden");
        amountSlider.classList.add("hidden");
        sliderOutput.classList.add("hidden");
    }
}

function updateActionButton(needToCall, player) {
    const val = parseInt(amountSlider.value, 10);
    const lastRaise = gameState ? gameState.last_raise || 20 : 20;
    const minRaise = needToCall + lastRaise;
    const isInvalid = val > needToCall && val < minRaise && val < player.chips;
    sliderOutput.classList.toggle("invalid", isInvalid);
    if (val === 0) {
        actionButton.textContent = "Check";
    } else if (val === player.chips) {
        actionButton.textContent = "All-In";
    } else if (val === needToCall) {
        actionButton.textContent = "Call";
    } else {
        actionButton.textContent = "Raise";
    }
}

async function onAction() {
    const val = parseInt(amountSlider.value, 10);
    const player = players.find(p => !p.is_bot && !p.folded && !p.all_in);
    if (!player) return;
    const needToCall = gameState.current_bet - player.round_bet;
    let action, amount;
    if (val === 0) {
        action = "check";
        amount = 0;
    } else if (val === player.chips) {
        action = "raise";
        amount = val;
    } else if (val === needToCall) {
        action = "call";
        amount = val;
    } else {
        action = "raise";
        amount = val;
    }
    try {
        const resp = await fetch(`${API_BASE}/act/`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                table_id: tableId,
                player_name: player.name,
                action,
                amount,
            }),
        });
        const data = await resp.json();
        if (data.ok) {
            applyState(data.state);
        }
    } catch (e) {
        console.error("action failed", e);
    }
}

function onFold() {
    const player = players.find(p => !p.is_bot && !p.folded && !p.all_in);
    if (!player) return;
    fetch(`${API_BASE}/act/`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            table_id: tableId,
            player_name: player.name,
            action: "fold",
            amount: 0,
        }),
    }).then(r => r.json()).then(data => {
        if (data.ok) applyState(data.state);
    });
}

function rotateSeat(ev) {
    const seat = ev.target.closest(".seat");
    const r = (parseInt(seat.dataset.rotation) + 90) % 360;
    seat.dataset.rotation = r;
    seat.style.transform = "rotate(" + r + "deg)";
}

function deletePlayer(ev) {
    const seat = ev.target.closest(".seat");
    seat.classList.add("hidden");
}

function enqueueNotification(msg) {
    pendingNotif.push(msg);
    if (!isNotifProcessing) showNextNotif();
}

function showNextNotif() {
    if (pendingNotif.length === 0) { isNotifProcessing = false; return; }
    isNotifProcessing = true;
    const msg = pendingNotif.shift();
    notifArr.unshift(msg);
    if (notifArr.length > MAX_ITEMS) notifArr.pop();
    const span = document.createElement("span");
    span.textContent = msg;
    notification.prepend(span);
    while (notification.childElementCount > MAX_ITEMS) {
        notification.removeChild(notification.lastChild);
    }
    setTimeout(showNextNotif, NOTIF_INTERVAL);
}

function init() {
    if (window.top !== window.self) {
        try { window.top.location.href = window.location.href; }
        catch { alert("No framing allowed."); throw new Error("No framing allowed."); }
    }
    document.addEventListener("touchstart", function () {}, false);
    startButton.addEventListener("click", startGame, false);
    actionButton.addEventListener("click", onAction, false);
    foldButton.addEventListener("click", onFold, false);
    amountSlider.addEventListener("input", () => {
        const player = players.find(p => !p.is_bot && !p.folded && !p.allIn);
        if (!player) return;
        const needToCall = gameState ? gameState.current_bet - player.round_bet : 0;
        updateActionButton(needToCall, player);
    });
    for (const ri of rotateIcons) ri.addEventListener("click", rotateSeat, false);
    for (const cb of closeButtons) cb.addEventListener("click", deletePlayer, false);

    const urlParams = new URLSearchParams(window.location.search);
    const tid = urlParams.get("table_id");
    if (tid) {
        tableId = tid;
        if (tableIdInput) tableIdInput.value = tid;
        const url = new URL(window.location);
        url.searchParams.set("table_id", tid);
        window.history.replaceState(null, "", url.toString());
        pollState();
    }
}

init();
