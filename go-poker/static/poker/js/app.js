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
const winprobToggle = document.querySelector("#winprob-toggle");
const winprobState = document.querySelector("#winprob-state");
const showdownOverlay = document.querySelector("#showdown-overlay");
const showdownWinnerName = document.querySelector("#showdown-winner-name");
const showdownHandName = document.querySelector("#showdown-hand-name");
const showdownCards = document.querySelector("#showdown-cards");
const showdownAmount = document.querySelector("#showdown-amount");
const showdownCountdownValue = document.querySelector("#showdown-countdown-value");
const showdownSkip = document.querySelector("#showdown-skip");
const showdownSeatList = document.querySelector("#showdown-seat-list");
const showdownSeatAddName = document.querySelector("#showdown-seat-add-name");
const showdownSeatAddBtn = document.querySelector("#showdown-seat-add-btn");
const communitySlots = document.querySelectorAll("#community-cards .cardslot");

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

// Showdown countdown state.
const COUNTDOWN_SECONDS = 5;
let countdownTimer = null;
let countdownDeadline = 0;
let countdownCurrentValue = -1;
let lastWinnerId = null;

// Win-probability admin toggle state. Loaded from /api/admin/wpmode on
// page load; toggled by clicking #winprob-toggle.
let winProbEnabled = false;
let winProbServiceConfigured = false;

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

// handNameClass maps a hand description (e.g. "Royal Flush", "Two Pair, A's & K's")
// to a CSS class slug for color coding.
function handNameClass(name) {
    if (!name) return "";
    const lower = name.toLowerCase();
    if (lower.includes("royal flush")) return "royal-flush";
    if (lower.includes("straight flush")) return "straight-flush";
    if (lower.includes("four of a kind")) return "four-of-a-kind";
    if (lower.includes("full house")) return "full-house";
    if (lower.includes("flush")) return "flush";
    if (lower.includes("straight")) return "straight";
    if (lower.includes("three of a kind")) return "three-of-a-kind";
    if (lower.includes("two pair")) return "two-pair";
    if (lower.includes("pair")) return "pair";
    if (lower.includes("high")) return "high-card";
    return "";
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

    // Show or refresh the showdown modal based on intermission state.
    handleShowdown(state);

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
    highlightCommunityCards(state);

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
            const showWP = (winProbEnabled || state.spectator_mode)
                && p.win_probability != null
                && !p.folded;
            if (showWP) {
                wpEl.textContent = `${Math.round(p.win_probability)}%`;
                wpEl.classList.remove("hidden");
            } else {
                wpEl.classList.add("hidden");
            }
        }

        // Hand name (post-flop only, when cards are revealed)
        const hnEl = seat.querySelector(".hand-name");
        if (hnEl) {
            const showHN = p.hand_name && !p.folded
                && (state.spectator_mode || state.open_cards_mode || winProbEnabled);
            if (showHN) {
                hnEl.textContent = p.hand_name;
                hnEl.classList.remove("hidden");
                hnEl.className = "hand-name " + handNameClass(p.hand_name);
            } else {
                hnEl.classList.add("hidden");
                hnEl.textContent = "";
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
            applyNotifications(data.state ? data.state.notifications : []);
            updateUIForPhase(data.state);
            const url = new URL(window.location);
            url.searchParams.set("table_id", tableId);
            window.history.replaceState(null, "", url.toString());

            if (data.state && data.state.game_started) {
                scheduleAdvance();
            }
            schedulePoll();
        }
    } catch (e) {
        console.error("start game failed", e);
        startButton.classList.remove("hidden");
    }
}

async function advanceGame() {
    if (!tableId) return;
    if (gameState && gameState.intermission) {
        // During intermission the showdown countdown drives the next hand.
        return;
    }
    try {
        const resp = await fetch(`${API_BASE}/advance/`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ table_id: tableId }),
        });
        if (!resp.ok) return;
        const data = await resp.json();
        if (data.ok && data.state) {
            previousVersion = data.version || (data.state.total_hands || 0);
            applyState(data.state);
            applyNotifications(data.state.notifications || []);
            updateUIForPhase(data.state);
            startButton.classList.toggle("hidden", data.state.game_started);

            if (data.state.game_started && !data.state.game_finished) {
                const needsHuman = (data.state.players || []).some(
                    p => !p.is_bot && !p.folded && !p.all_in
                );
                if (!needsHuman) {
                    scheduleAdvance();
                }
            }
        }
    } catch (e) {
        console.warn("advance failed", e);
    }
}

function scheduleAdvance() {
    setTimeout(advanceGame, 1200);
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
    if (!state || !state.game_started || state.game_finished || state.intermission) {
        foldButton.classList.add("hidden");
        actionButton.classList.add("hidden");
        amountSlider.classList.add("hidden");
        sliderOutput.classList.add("hidden");
        startButton.classList.toggle("hidden", !!(state && state.intermission));
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

// ============================================================
// Showdown modal + countdown + seat editing
// ============================================================

function handleShowdown(state) {
    if (state.intermission && state.winner) {
        const id = (state.winner.name || "") + "|" + (state.winner.amount || 0) + "|" + (state.intermission_started_at || 0);
        if (id !== lastWinnerId) {
            lastWinnerId = id;
            showShowdownModal(state);
        } else {
            refreshSeatEditor(state);
        }
        startCountdown(state);
        renderSeatEditor(state);
    } else {
        hideShowdownModal();
    }
}

function showShowdownModal(state) {
    if (!showdownOverlay) return;
    showdownOverlay.classList.remove("hidden");

    const w = state.winner || {};
    if (showdownWinnerName) {
        if (w.is_tie && w.tied_with && w.tied_with.length > 1) {
            showdownWinnerName.textContent = w.tied_with.join(" & ") + " tie";
        } else {
            showdownWinnerName.textContent = (w.name || "Winner") + " wins";
        }
    }
    if (showdownHandName) showdownHandName.textContent = w.hand_name || "";

    if (showdownCards) {
        showdownCards.innerHTML = "";
        const cards = w.winning_cards || [];
        cards.forEach((c, i) => {
            const el = document.createElement("div");
            el.className = "showdown-card " + suitClass(c);
            el.textContent = formatCardText(c);
            el.style.animationDelay = (0.4 + i * 0.2) + "s";
            showdownCards.appendChild(el);
        });
    }

    if (showdownAmount) {
        showdownAmount.textContent = "+" + (w.amount || 0);
        showdownAmount.classList.remove("pulse");
        // restart the animation
        void showdownAmount.offsetWidth;
        showdownAmount.classList.add("pulse");
    }

    animateChipsToWinner(state);
}

function hideShowdownModal() {
    if (!showdownOverlay) return;
    showdownOverlay.classList.add("hidden");
    lastWinnerId = null;
    stopCountdown();
    clearCommunityHighlights();
}

function suitClass(card) {
    if (!card || card.length < 2) return "";
    const suit = card[1].toLowerCase();
    if (suit === "h") return "hearts";
    if (suit === "d") return "diamonds";
    if (suit === "c") return "clubs";
    if (suit === "s") return "spades";
    return "";
}

function formatCardText(code) {
    if (!code || code.length < 2) return code;
    const rank = code[0] === "T" ? "10" : code[0];
    const suit = code[1].toUpperCase();
    const suitMap = { C: "\u2663", D: "\u2666", H: "\u2665", S: "\u2660" };
    return rank + (suitMap[suit] || suit);
}

function animateChipsToWinner(state) {
    const w = state.winner;
    if (!w) return;
    const seatEl = document.querySelectorAll(".seat:not(.hidden)")[w.seat_index || 0];
    const potEl = document.querySelector("#pot") || document.querySelector("#table-middle");
    if (!seatEl || !potEl) return;
    const potRect = potEl.getBoundingClientRect();
    const seatRect = seatEl.getBoundingClientRect();
    const dx = seatRect.left + seatRect.width / 2 - (potRect.left + potRect.width / 2);
    const dy = seatRect.top + seatRect.height / 2 - (potRect.top + potRect.height / 2);
    const chipCount = Math.min(8, Math.max(3, Math.ceil((w.amount || 0) / 200)));
    for (let i = 0; i < chipCount; i++) {
        const chip = document.createElement("div");
        chip.className = "flying-chip";
        const jitter = (i - chipCount / 2) * 18;
        chip.style.left = (potRect.left + potRect.width / 2 + jitter) + "px";
        chip.style.top = (potRect.top + potRect.height / 2) + "px";
        chip.style.setProperty("--dx", dx + jitter + "px");
        chip.style.setProperty("--dy", dy + "px");
        chip.style.animationDelay = (1.4 + i * 0.05) + "s";
        document.body.appendChild(chip);
        setTimeout(() => chip.remove(), 2400);
    }
}

function highlightCommunityCards(state) {
    if (!communitySlots) return;
    const winningCards = (state.winner && state.winner.winning_cards) || [];
    if (!state.intermission || winningCards.length === 0) {
        communitySlots.forEach(s => {
            s.classList.remove("blurred");
            s.classList.remove("highlight");
        });
        return;
    }
    const winningSet = new Set(winningCards);
    communitySlots.forEach((slot) => {
        const img = slot.querySelector("img");
        if (!img) return;
        const src = img.getAttribute("src") || "";
        const code = src.split("/").pop().replace(".svg", "");
        if (winningSet.has(code)) {
            slot.classList.add("highlight");
            slot.classList.remove("blurred");
        } else {
            slot.classList.add("blurred");
            slot.classList.remove("highlight");
        }
    });
}

function clearCommunityHighlights() {
    if (!communitySlots) return;
    communitySlots.forEach(s => {
        s.classList.remove("blurred");
        s.classList.remove("highlight");
    });
}

function startCountdown(state) {
    if (!showdownCountdownValue) return;
    const startedAt = state.intermission_started_at || Date.now();
    countdownDeadline = startedAt + COUNTDOWN_SECONDS * 1000;
    if (countdownTimer) clearInterval(countdownTimer);
    countdownTimer = setInterval(tickCountdown, 250);
    tickCountdown();
    if (showdownSkip && !showdownSkip.dataset.bound) {
        showdownSkip.addEventListener("click", requestNextHand);
        showdownSkip.dataset.bound = "1";
    }
}

function stopCountdown() {
    if (countdownTimer) clearInterval(countdownTimer);
    countdownTimer = null;
    countdownCurrentValue = -1;
}

function tickCountdown() {
    const remainingMs = countdownDeadline - Date.now();
    const seconds = Math.max(0, Math.ceil(remainingMs / 1000));
    if (seconds !== countdownCurrentValue) {
        countdownCurrentValue = seconds;
        if (showdownCountdownValue) showdownCountdownValue.textContent = seconds;
        if (showdownCountdownValue) {
            showdownCountdownValue.classList.remove("tick");
            void showdownCountdownValue.offsetWidth;
            showdownCountdownValue.classList.add("tick");
        }
    }
    if (remainingMs <= 0) {
        stopCountdown();
        requestNextHand();
    }
}

async function requestNextHand() {
    stopCountdown();
    try {
        const resp = await fetch(`${API_BASE}/game/next/`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ table_id: tableId }),
        });
        if (!resp.ok) return;
        const data = await resp.json();
        if (data.ok && data.state) {
            applyState(data.state);
        }
    } catch (e) {
        console.warn("next hand failed", e);
    }
}

function renderSeatEditor(state) {
    if (!showdownSeatList) return;
    showdownSeatList.innerHTML = "";
    (state.players || []).forEach((p, i) => {
        const chip = document.createElement("div");
        chip.className = "showdown-seat-chip";
        chip.innerHTML = `<span>${p.name}</span><button data-seat="${i}" title="Remove">&times;</button>`;
        chip.querySelector("button").addEventListener("click", () => removeSeat(i));
        showdownSeatList.appendChild(chip);
    });
}

function refreshSeatEditor(state) {
    // Re-render in case players changed via another client.
    if (showdownSeatList && showdownSeatList.childElementCount !== (state.players || []).length) {
        renderSeatEditor(state);
    }
}

async function addSeat() {
    if (!showdownSeatAddName) return;
    const name = showdownSeatAddName.value.trim();
    if (!name) return;
    try {
        const resp = await fetch(`${API_BASE}/seats/join/`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ table_id: tableId, name }),
        });
        if (!resp.ok) return;
        const data = await resp.json();
        if (data.ok && data.state) {
            showdownSeatAddName.value = "";
            applyState(data.state);
        }
    } catch (e) {
        console.warn("add seat failed", e);
    }
}

async function removeSeat(seatIndex) {
    try {
        const resp = await fetch(`${API_BASE}/seats/leave/`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ table_id: tableId, seat_index: seatIndex }),
        });
        if (!resp.ok) return;
        const data = await resp.json();
        if (data.ok && data.state) {
            applyState(data.state);
        }
    } catch (e) {
        console.warn("remove seat failed", e);
    }
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
    if (winprobToggle) {
        winprobToggle.addEventListener("click", toggleWinProb, false);
    }
    if (showdownSeatAddBtn) {
        showdownSeatAddBtn.addEventListener("click", addSeat, false);
    }
    if (showdownSeatAddName) {
        showdownSeatAddName.addEventListener("keydown", (e) => {
            if (e.key === "Enter") { e.preventDefault(); addSeat(); }
        });
    }
    fetchAdminState();

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

async function fetchAdminState() {
    try {
        const resp = await fetch(`${API_BASE}/admin/wpmode`, { method: "GET" });
        if (!resp.ok) return;
        const data = await resp.json();
        winProbEnabled = !!data.enabled;
        winProbServiceConfigured = !!data.service_configured;
        renderAdminToggle();
    } catch (e) {
        console.warn("admin state fetch failed", e);
    }
}

function renderAdminToggle() {
    if (!winprobToggle) return;
    if (!winProbServiceConfigured) {
        winprobToggle.classList.add("hidden");
        return;
    }
    winprobToggle.classList.remove("hidden");
    winprobToggle.classList.toggle("on", winProbEnabled);
    winprobToggle.classList.toggle("off", !winProbEnabled);
    if (winprobState) winprobState.textContent = winProbEnabled ? "on" : "off";
}

async function toggleWinProb() {
    const next = !winProbEnabled;
    try {
        const resp = await fetch(`${API_BASE}/admin/wpmode`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ enabled: next }),
        });
        if (!resp.ok) throw new Error(`status ${resp.status}`);
        const data = await resp.json();
        winProbEnabled = !!data.enabled;
        renderAdminToggle();
        // Re-render current state so percentages appear / disappear.
        if (gameState) applyState(gameState);
        // Kick a fresh fetch so we get fresh win_probability values
        // (or nulls).
        pollState();
    } catch (e) {
        console.warn("toggle failed", e);
    }
}

init();
