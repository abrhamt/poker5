"""
Game engine — direct port of the core game logic from app.js

Manages the full poker game lifecycle: dealing, betting rounds,
showdown evaluation, side pots, chip transfers, and state management.
"""

import json
import random
import string
from .hand_evaluator import Hand
from .bot import choose_bot_action

RANKS = ['2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A']
SUITS = ['C', 'D', 'H', 'S']
FULL_DECK = [r + s for s in SUITS for r in RANKS]

PHASES = ['preflop', 'flop', 'turn', 'river', 'showdown']

MAX_NOTIFICATIONS = 8


def shuffle_deck(deck):
    d = deck[:]
    random.shuffle(d)
    return d


def get_hands_played_bucket(hand_count):
    if hand_count < 20:
        return '<20'
    if hand_count <= 25:
        return '20-25'
    if hand_count <= 30:
        return '26-30'
    if hand_count <= 35:
        return '31-35'
    if hand_count <= 40:
        return '36-40'
    if hand_count <= 45:
        return '41-45'
    if hand_count <= 50:
        return '46-50'
    if hand_count <= 55:
        return '51-55'
    if hand_count <= 60:
        return '56-60'
    return '>60'


def combination_count(n, k):
    if k < 0 or k > n:
        return 0
    kk = min(k, n - k)
    result = 1
    for i in range(1, kk + 1):
        result = result * (n - kk + i) // i
    return result


def generate_table_id():
    return ''.join(random.choices(string.ascii_lowercase + string.digits, k=6))


class GameEngine:
    def __init__(self, game_model=None):
        self.game = game_model
        self.deck = []
        self.card_graveyard = []
        self.players = []
        self.phase_index = 0
        self.pot = 0
        self.current_bet = 0
        self.last_raise = 20
        self.raises_this_round = 0
        self.small_blind = 10
        self.big_blind = 20
        self.dealer_orbit_count = -1
        self.initial_dealer_name = None
        self.game_started = False
        self.game_finished = False
        self.open_cards_mode = False
        self.spectator_mode = False
        self.total_hands = 0
        self.notifications = []
        self.phase = 'preflop'
        self.current_player_index = 0
        self.community_cards = []

        if game_model:
            self.load_from_model(game_model)

    def load_from_model(self, game_model):
        self.game = game_model
        self.deck = json.loads(game_model.deck) if game_model.deck else []
        self.card_graveyard = json.loads(game_model.card_graveyard) if game_model.card_graveyard else []
        self.community_cards = json.loads(game_model.community_cards) if game_model.community_cards else []
        self.phase_index = PHASES.index(game_model.phase) if game_model.phase in PHASES else 0
        self.phase = game_model.phase
        self.pot = game_model.pot
        self.current_bet = game_model.current_bet
        self.last_raise = game_model.last_raise
        self.small_blind = game_model.small_blind
        self.big_blind = game_model.big_blind
        self.raises_this_round = game_model.raises_this_round
        self.dealer_orbit_count = game_model.dealer_orbit_count
        self.initial_dealer_name = game_model.initial_dealer_name
        self.game_started = game_model.game_started
        self.game_finished = game_model.game_finished
        self.open_cards_mode = game_model.open_cards_mode
        self.spectator_mode = game_model.spectator_mode
        self.total_hands = game_model.total_hands
        self.notifications = json.loads(game_model.notifications) if game_model.notifications else []
        self.players = []
        for p in game_model.players.all():
            self.players.append(self._player_from_model(p))

    def _player_from_model(self, p_model):
        stats = json.loads(p_model.stats_data) if p_model.stats_data else {}
        return {
            'id': p_model.id,
            'name': p_model.name,
            'seat_index': p_model.seat_index,
            'is_bot': p_model.is_bot,
            'chips': p_model.chips,
            'round_bet': p_model.round_bet,
            'total_bet': p_model.total_bet,
            'folded': p_model.folded,
            'all_in': p_model.all_in,
            'dealer': p_model.is_dealer,
            'small_blind': p_model.is_small_blind,
            'big_blind': p_model.is_big_blind,
            'cards': [p_model.card1 or '1B', p_model.card2 or '1B'],
            'win_probability': p_model.win_probability,
            'stats': stats,
            'bot_line': {
                'preflop_aggressor': False,
                'cbet_intent': None,
                'barrel_intent': None,
                'cbet_made': False,
                'barrel_made': False,
                'non_value_aggression_made': False,
            },
        }

    def save_state(self):
        if not self.game:
            return
        from .models import Player
        self.game.deck = json.dumps(self.deck)
        self.game.card_graveyard = json.dumps(self.card_graveyard)
        self.game.community_cards = json.dumps(self.community_cards)
        self.game.phase = PHASES[self.phase_index] if self.phase_index < len(PHASES) else 'preflop'
        self.game.pot = self.pot
        self.game.current_bet = self.current_bet
        self.game.last_raise = self.last_raise
        self.game.small_blind = self.small_blind
        self.game.big_blind = self.big_blind
        self.game.raises_this_round = self.raises_this_round
        self.game.dealer_orbit_count = self.dealer_orbit_count
        self.game.initial_dealer_name = self.initial_dealer_name
        self.game.game_started = self.game_started
        self.game.game_finished = self.game_finished
        self.game.open_cards_mode = self.open_cards_mode
        self.game.spectator_mode = self.spectator_mode
        self.game.total_hands = self.total_hands
        self.game.notifications = json.dumps(self.notifications[-MAX_NOTIFICATIONS:])
        self.game.version += 1
        self.game.save()

        for p_data in self.players:
            p_model, _ = Player.objects.update_or_create(
                game=self.game,
                seat_index=p_data['seat_index'],
                defaults={
                    'name': p_data['name'],
                    'is_bot': p_data['is_bot'],
                    'chips': p_data['chips'],
                    'round_bet': p_data['round_bet'],
                    'total_bet': p_data['total_bet'],
                    'folded': p_data['folded'],
                    'all_in': p_data['all_in'],
                    'is_dealer': p_data['dealer'],
                    'is_small_blind': p_data['small_blind'],
                    'is_big_blind': p_data['big_blind'],
                    'card1': p_data['cards'][0] if len(p_data['cards']) > 0 else None,
                    'card2': p_data['cards'][1] if len(p_data['cards']) > 0 else None,
                    'win_probability': p_data.get('win_probability'),
                    'stats_data': json.dumps(p_data['stats']),
                },
            )

    def to_dict(self):
        return {
            'phase': PHASES[self.phase_index] if self.phase_index < len(PHASES) else None,
            'pot': self.pot,
            'current_bet': self.current_bet,
            'last_raise': self.last_raise,
            'small_blind': self.small_blind,
            'big_blind': self.big_blind,
            'raises_this_round': self.raises_this_round,
            'dealer_orbit_count': self.dealer_orbit_count,
            'community_cards': self.community_cards,
            'players': [{
                'name': p['name'],
                'chips': p['chips'],
                'round_bet': p['round_bet'],
                'total_bet': p['total_bet'],
                'folded': p['folded'],
                'all_in': p['all_in'],
                'is_bot': p['is_bot'],
                'dealer': p['dealer'],
                'small_blind': p['small_blind'],
                'big_blind': p['big_blind'],
                'cards': p['cards'] if self.open_cards_mode or self.spectator_mode else ['1B', '1B'],
                'seat_index': p['seat_index'],
                'stats': p['stats'],
                'win_probability': p.get('win_probability'),
            } for p in self.players],
            'game_started': self.game_started,
            'game_finished': self.game_finished,
            'open_cards_mode': self.open_cards_mode,
            'spectator_mode': self.spectator_mode,
            'total_hands': self.total_hands,
            'notifications': self.notifications[-MAX_NOTIFICATIONS:],
            'timestamp': None,
        }

    def init_game(self, player_names):
        self.deck = shuffle_deck(FULL_DECK)
        self.card_graveyard = []
        self.players = []
        self.game_started = False
        self.game_finished = False
        self.total_hands = 0
        self.notifications = []
        self.phase_index = 0
        self.pot = 0
        self.current_bet = 0
        self.last_raise = 20
        self.small_blind = 10
        self.big_blind = 20
        self.dealer_orbit_count = -1
        self.initial_dealer_name = None
        self.open_cards_mode = False
        self.spectator_mode = False
        self.community_cards = []

        for idx, name in enumerate(player_names):
            is_bot = name.lower().startswith('bot')
            stats = {
                'hands': 0, 'hands_won': 0, 'vpip': 0, 'pfr': 0,
                'calls': 0, 'aggressive_acts': 0, 'showdowns': 0,
                'showdowns_won': 0, 'folds': 0, 'folds_preflop': 0,
                'folds_postflop': 0, 'allins': 0,
            }
            self.players.append({
                'name': name,
                'seat_index': idx,
                'is_bot': is_bot,
                'chips': 2000,
                'round_bet': 0,
                'total_bet': 0,
                'folded': False,
                'all_in': False,
                'dealer': False,
                'small_blind': False,
                'big_blind': False,
                'cards': ['1B', '1B'],
                'win_probability': None,
                'stats': stats,
                'bot_line': {
                    'preflop_aggressor': False,
                    'cbet_intent': None,
                    'barrel_intent': None,
                    'cbet_made': False,
                    'barrel_made': False,
                    'non_value_aggression_made': False,
                },
            })

        self.game_started = True

    def add_notification(self, msg):
        self.notifications.append(msg)
        if len(self.notifications) > MAX_NOTIFICATIONS:
            self.notifications = self.notifications[-MAX_NOTIFICATIONS:]

    def start_hand(self):
        self.game_started = True
        self.total_hands += 1
        self.phase_index = 0
        self.pot = 0
        self.current_bet = 0
        self.last_raise = self.big_blind
        self.raises_this_round = 0
        self.community_cards = []

        for p in self.players:
            p['folded'] = False
            p['all_in'] = False
            p['total_bet'] = 0
            p['round_bet'] = 0
            p['win_probability'] = None
            p['cards'] = ['1B', '1B']
            p['bot_line'] = {
                'preflop_aggressor': False,
                'cbet_intent': None,
                'barrel_intent': None,
                'cbet_made': False,
                'barrel_made': False,
                'non_value_aggression_made': False,
            }

        active_players = [p for p in self.players if p['chips'] > 0]
        for p in self.players:
            if p['chips'] <= 0:
                self.add_notification(f"{p['name']} is out of the game!")

        self.players = active_players

        if len(self.players) == 0:
            return False

        human_count = sum(1 for p in self.players if not p['is_bot'])
        self.open_cards_mode = human_count == 1
        self.spectator_mode = human_count == 0

        if not any(p['name'] == self.initial_dealer_name for p in self.players):
            self.initial_dealer_name = self.players[0]['name'] if self.players else None
            self.dealer_orbit_count = -1

        for p in self.players:
            p['stats']['hands'] += 1

        if len(self.players) == 1:
            champion = self.players[0]
            self.add_notification(f"{champion['name']} wins the game!")
            self.game_finished = True
            return False

        self._set_dealer()
        self._set_blinds()
        self._deal_cards()
        self.save_state()
        self._start_betting_round()
        return True

    def _set_dealer(self):
        if all(not p['dealer'] for p in self.players):
            idx = random.randint(0, len(self.players) - 1)
            self.players[idx]['dealer'] = True
            self.initial_dealer_name = self.players[idx]['name']
        else:
            dealer_idx = next(i for i, p in enumerate(self.players) if p['dealer'])
            self.players[dealer_idx]['dealer'] = False
            next_idx = (dealer_idx + 1) % len(self.players)
            self.players[next_idx]['dealer'] = True

        while not self.players[0]['dealer']:
            self.players.append(self.players.pop(0))

        self.add_notification(f"{self.players[0]['name']} is Dealer.")

    def _set_blinds(self):
        if self.players[0]['name'] == self.initial_dealer_name:
            self.dealer_orbit_count += 1
            if self.dealer_orbit_count > 0 and self.dealer_orbit_count % 2 == 0:
                self.small_blind *= 2
                self.big_blind *= 2
                self.add_notification(f"Blinds are now {self.small_blind}/{self.big_blind}.")

        for p in self.players:
            p['small_blind'] = False
            p['big_blind'] = False

        sb_idx = 1 if len(self.players) > 2 else 0
        bb_idx = 2 if len(self.players) > 2 else 1

        sb_bet = self._place_bet(self.players[sb_idx], self.small_blind)
        bb_bet = self._place_bet(self.players[bb_idx], self.big_blind)

        self.add_notification(f"{self.players[sb_idx]['name']} posted small blind of {sb_bet}.")
        self.add_notification(f"{self.players[bb_idx]['name']} posted big blind of {bb_bet}.")

        self.pot += sb_bet + bb_bet
        self.players[sb_idx]['small_blind'] = True
        self.players[bb_idx]['big_blind'] = True
        self.current_bet = self.big_blind
        self.last_raise = self.big_blind

    def _place_bet(self, player, amount):
        bet = min(amount, player['chips'])
        player['round_bet'] += bet
        player['total_bet'] += bet
        player['chips'] -= bet
        if player['chips'] == 0:
            player['all_in'] = True
        return bet

    def _deal_cards(self):
        self.deck = shuffle_deck(FULL_DECK)
        self.card_graveyard = []
        for p in self.players:
            if len(self.deck) >= 2:
                p['cards'][0] = self.deck.pop(0)
                p['cards'][1] = self.deck.pop(0)
            self.card_graveyard.extend(p['cards'])

    def _deal_community_cards(self, count):
        if len(self.deck) <= count:
            return
        self.deck.pop(0)
        for _ in range(count):
            if self.deck:
                card = self.deck.pop(0)
                self.community_cards.append(card)
                self.card_graveyard.append(card)

    def _start_betting_round(self):
        if self.phase_index > 0:
            self.current_bet = 0
            self.last_raise = self.big_blind
            for p in self.players:
                p['round_bet'] = 0

        active_players = [p for p in self.players if not p['folded']]
        actionable = [p for p in active_players if not p['all_in']]

        if len(active_players) <= 1 or len(actionable) <= 1:
            self._advance_phase()
            return

        if self.phase_index == 0:
            bb_idx = next(i for i, p in enumerate(self.players) if p['big_blind'])
            self.current_player_index = (bb_idx + 1) % len(self.players)
        else:
            dealer_players = [(i, p) for i, p in enumerate(self.players) if p['dealer']]
            if dealer_players:
                dealer_idx = dealer_players[0][0]
            else:
                active = [p for p in self.players if not p['folded']]
                dealer_idx = self.players.index(active[0]) if active else 0
            self.current_player_index = (dealer_idx + 1) % len(self.players)

        self.raises_this_round = 0
        self.save_state()

    def _process_current_player(self):
        if self.game_finished:
            return

        active_players = [p for p in self.players if not p['folded']]
        actionable = [p for p in active_players if not p['all_in']]

        if len(active_players) <= 1 or len(actionable) == 0:
            self._advance_phase()
            return

        idx = self.current_player_index % len(self.players)
        player = self.players[idx]

        if player['folded'] or player['all_in']:
            self.current_player_index += 1
            self.save_state()
            return

        if player['round_bet'] >= self.current_bet:
            cycles = sum(1 for p in self.players if not p['folded'] and not p['all_in'])
            if cycle_check(self, idx, cycles):
                self._advance_phase()
                return

        if player['is_bot']:
            self._process_bot_action(player)
        else:
            self.save_state()

    def advance_one_step(self):
        if self.game_finished:
            return False

        # Start a new hand if the previous one ended
        if not self.game_started:
            self.start_hand()
            return True

        active_players = [p for p in self.players if not p['folded']]
        actionable = [p for p in active_players if not p['all_in']]

        if len(active_players) <= 1 or len(actionable) == 0:
            self._advance_phase()
            return True

        idx = self.current_player_index % len(self.players)
        player = self.players[idx]

        if player['folded'] or player['all_in']:
            self.current_player_index += 1
            self.save_state()
            return True

        if player['round_bet'] >= self.current_bet:
            cycles = sum(1 for p in self.players if not p['folded'] and not p['all_in'])
            if cycle_check(self, idx, cycles):
                self._advance_phase()
                return True

        self._process_current_player()
        return True

    def _process_bot_action(self, player):
        ctx = {
            'current_bet': self.current_bet,
            'pot': self.pot,
            'small_blind': self.small_blind,
            'big_blind': self.big_blind,
            'raises_this_round': self.raises_this_round,
            'current_phase_index': self.phase_index,
            'players': self.players,
            'last_raise': self.last_raise,
            'community_cards': self.community_cards,
        }
        decision = choose_bot_action(player, ctx)
        self._apply_decision(player, decision)
        self.current_player_index += 1
        self.save_state()

    def _apply_decision(self, player, decision):
        need_to_call = self.current_bet - player['round_bet']
        action = decision['action']

        self._update_stats(player, action)

        if action == 'fold':
            player['folded'] = True
            self.add_notification(f"{player['name']} folded.")
        elif action == 'check':
            self.add_notification(f"{player['name']} checked.")
        elif action == 'call':
            amt = self._place_bet(player, decision.get('amount', need_to_call))
            self.pot += amt
            self.add_notification(f"{player['name']} called {amt}.")
        elif action == 'raise':
            bet = decision.get('amount', need_to_call + self.last_raise)
            amt = self._place_bet(player, bet)
            if amt > need_to_call:
                self.current_bet = player['round_bet']
                self.last_raise = amt - need_to_call
                self.raises_this_round += 1
            self.pot += amt
            self.add_notification(f"{player['name']} raised to {amt}.")

    def _update_stats(self, player, action):
        if self.phase_index == 0:
            if action in ('call', 'raise', 'allin'):
                player['stats']['vpip'] += 1
            if action in ('raise', 'allin'):
                player['stats']['pfr'] += 1
        else:
            if action in ('raise', 'allin'):
                player['stats']['aggressive_acts'] += 1
            if action == 'call':
                player['stats']['calls'] += 1
        if action in ('raise', 'allin') and self.phase_index == 0:
            for p in self.players:
                p['bot_line']['preflop_aggressor'] = False
            player['bot_line']['preflop_aggressor'] = True
        if action == 'allin':
            player['stats']['allins'] += 1
        if action == 'fold':
            player['stats']['folds'] += 1
            if self.phase_index == 0:
                player['stats']['folds_preflop'] += 1
            else:
                player['stats']['folds_postflop'] += 1

    def _advance_phase(self):
        active_players = [p for p in self.players if not p['folded']]
        if len(active_players) <= 1:
            self._do_showdown()
            return

        self.phase_index += 1

        if self.phase_index >= len(PHASES):
            self._do_showdown()
            return

        phase = PHASES[self.phase_index]
        if phase == 'flop':
            self._deal_community_cards(3)
            self.add_notification("Flop (3 cards) dealt.")
        elif phase == 'turn':
            self._deal_community_cards(1)
            self.add_notification("Turn (4th card) dealt.")
        elif phase == 'river':
            self._deal_community_cards(1)
            self.add_notification("River (5th card) dealt.")
        elif phase == 'showdown':
            self._do_showdown()
            return

        self.save_state()
        self._start_betting_round()

    def _do_showdown(self):
        for p in self.players:
            p['round_bet'] = 0

        active_players = [p for p in self.players if not p['folded']]
        contributors = [p for p in self.players if p['total_bet'] > 0]

        had_showdown = len(active_players) > 1
        if had_showdown:
            for p in active_players:
                p['stats']['showdowns'] += 1

        if len(active_players) == 1:
            winner = active_players[0]
            winner['stats']['hands_won'] += 1
            winner['chips'] += self.pot
            self.add_notification(f"{winner['name']} wins {self.pot}!")
            self.pot = 0
            self.game_started = False
            self.save_state()
            return

        contenders = contributors[:]
        side_pots = self._build_side_pots(contenders)
        self._resolve_side_pots(side_pots, active_players, had_showdown)

    def _build_side_pots(self, contenders):
        sorted_contenders = sorted(contenders, key=lambda p: p['total_bet'])
        side_pots = []
        prev = 0
        for c in sorted_contenders:
            lvl = c['total_bet']
            diff = lvl - prev
            if diff > 0:
                eligible = sorted_contenders[sorted_contenders.index(c):]
                side_pots.append({
                    'amount': diff * len(eligible),
                    'eligible': eligible[:],
                })
                prev = lvl
        i = 0
        while i < len(side_pots) - 1:
            elig_a = [p for p in side_pots[i]['eligible'] if not p['folded']]
            elig_b = [p for p in side_pots[i + 1]['eligible'] if not p['folded']]
            if len(elig_a) == len(elig_b) and all(p in elig_b for p in elig_a):
                side_pots[i]['amount'] += side_pots[i + 1]['amount']
                side_pots.pop(i + 1)
            else:
                i += 1
        return side_pots

    def _resolve_side_pots(self, side_pots, active_players, had_showdown):
        winners_set = []
        for sp in side_pots:
            sp_eligible = [p for p in sp['eligible'] if not p['folded']]
            if len(sp_eligible) == 1:
                sole = sp_eligible[0]
                sole['chips'] += sp['amount']
                if sole not in winners_set:
                    sole['stats']['hands_won'] += 1
                    if had_showdown:
                        sole['stats']['showdowns_won'] += 1
                    winners_set.append(sole)
                self.add_notification(f"{sole['name']} wins {sp['amount']}.")
                continue

            sp_hands = []
            for p in sp_eligible:
                seven = [p['cards'][0], p['cards'][1]] + list(self.community_cards)
                try:
                    hand_obj = Hand.solve(seven)
                    sp_hands.append({'player': p, 'hand_obj': hand_obj})
                except Exception:
                    continue

            if not sp_hands:
                continue

            winners = Hand.winners([h['hand_obj'] for h in sp_hands])
            share = sp['amount'] // len(winners)
            remainder = sp['amount'] - share * len(winners)

            for w in winners:
                entry = next(h for h in sp_hands if h['hand_obj'] == w)
                payout = share + (1 if remainder > 0 else 0)
                if remainder > 0:
                    remainder -= 1
                entry['player']['chips'] += payout
                if entry['player'] not in winners_set:
                    entry['player']['stats']['hands_won'] += 1
                    if had_showdown:
                        entry['player']['stats']['showdowns_won'] += 1
                    winners_set.append(entry['player'])

            if len(winners) == 1:
                e = next(h for h in sp_hands if h['hand_obj'] == winners[0])
                self.add_notification(f"{e['player']['name']} wins {sp['amount']} with {winners[0].name}.")
            else:
                names = [next(h['player']['name'] for h in sp_hands if h['hand_obj'] == w) for w in winners]
                self.add_notification(f"{' & '.join(names)} split {sp['amount']}.")

        self.pot = 0
        self.game_started = False
        self.save_state()

    def human_action(self, player_name, action, amount=0):
        player = next((p for p in self.players if p['name'] == player_name and not p['folded']), None)
        if not player:
            return False

        if player['is_bot']:
            return False

        if player['all_in']:
            return False

        decision = {'action': action}
        if action in ('call', 'raise'):
            decision['amount'] = amount

        self._apply_decision(player, decision)
        self.current_player_index += 1
        self.save_state()

        if self.game_finished:
            return True

        active_players = [p for p in self.players if not p['folded']]
        actionable = [p for p in active_players if not p['all_in']]

        if len(active_players) <= 1 or len(actionable) <= 1:
            self._advance_phase()
        else:
            self._process_current_player()
        return True

    def next_hand(self):
        if not self.game_finished:
            self.start_hand()


class GameState:
    def __init__(self):
        self.engines = {}

    def get_or_create(self, table_id):
        from .models import Game
        game_model, created = Game.objects.get_or_create(
            table_id=table_id,
            defaults={
                'small_blind': 10,
                'big_blind': 20,
                'last_raise': 20,
            },
        )
        if table_id not in self.engines:
            self.engines[table_id] = GameEngine(game_model)
        return self.engines[table_id], created


def cycle_check(engine, idx, cycles):
    player = engine.players[idx % len(engine.players)]
    if player['folded'] or player['all_in']:
        return False
    if player['round_bet'] >= engine.current_bet:
        if engine.phase_index > 0 and engine.current_bet == 0:
            return False
        return cycles >= sum(1 for p in engine.players if not p['folded'] and not p['all_in'])
    return False
