"""
Bot AI — direct port of bot.js

Winner-take-all tournament bot decisions using Chen score preflop,
postflop hand evaluation via pokersolver, pot odds / SPR / M-ratio analysis,
opponent tracking, bet sizing, bluff/steal logic.
"""

import math
import random
from .hand_evaluator import Card, Hand

SUIT_SYMBOLS = {'C': '\u2663', 'D': '\u2666', 'H': '\u2665', 'S': '\u2660'}

MAX_RAISES_PER_ROUND = 3
RERAISE_RATIO_STEP = 0.12
RERAISE_VALUE_RATIO = 0.34
RERAISE_TOP_PAIR_RATIO = 0.32
STRENGTH_TIE_DELTA = 0.25
ODDS_TIE_DELTA = 0.02
OPPONENT_THRESHOLD = 3
AGG_FACTOR = 0.1
THRESHOLD_FACTOR = 0.3
MIN_HANDS_FOR_WEIGHT = 10
WEIGHT_GROWTH = 10
ALLIN_HAND_PREFLOP = 0.85
ALLIN_HAND_POSTFLOP = 0.38

M_RATIO_DEAD_MAX = 1
M_RATIO_RED_MAX = 5
M_RATIO_ORANGE_MAX = 10
M_RATIO_YELLOW_MAX = 20
DEAD_PUSH_RATIO = 0.35
RED_PUSH_RATIO = 0.7
RED_CALL_RATIO = 0.85
ORANGE_PUSH_RATIO = 0.6
ORANGE_CALL_RATIO = 0.8
YELLOW_RAISE_RATIO = 0.6
YELLOW_CALL_RATIO = 0.7
YELLOW_SHOVE_RATIO = 0.85
PREMIUM_PREFLOP_RATIO = 0.8
PREMIUM_POSTFLOP_RATIO = 0.55
GREEN_MAX_STACK_BET = 0.25
CHIP_LEADER_RAISE_DELTA = 0.05
SHORTSTACK_CALL_DELTA = 0.05
SHORTSTACK_RELATIVE = 0.6
MIN_PREFLOP_BLUFF_RATIO = 0.45
COMMIT_SPR_MIN = 1.5
COMMIT_SPR_MAX = 5.5
COMMIT_INVEST_START = 0.1
COMMIT_INVEST_END = 0.6
COMMIT_CALL_RATIO_REF = 0.25
COMMITMENT_PENALTY_MAX = 0.25
POSTFLOP_CALL_BARRIER = 0.16
ELIMINATION_RISK_START = 0.25
ELIMINATION_RISK_FULL = 0.8
ELIMINATION_PENALTY_MAX = 0.25

BOT_ACTION_DELAY = 3.0


def format_card(code):
    return code[0].replace('T', '10') + SUIT_SYMBOLS.get(code[1], code[1])


def round_to_10(x):
    return round(x / 10) * 10


def hand_tiebreaker(hand_obj):
    base = 15.0
    value = 0.0
    factor = 1.0 / base
    for card in hand_obj.cards:
        value += card.rank * factor
        factor /= base
    return value


def get_solved_hand_score(hand_obj):
    if not hand_obj:
        return 0
    return hand_obj.rank + hand_tiebreaker(hand_obj)


def solved_hand_uses_hole_cards(hole, hand_obj):
    if not hand_obj:
        return False
    hole_cards = [Card(c) for c in hole]
    for card in hand_obj.cards:
        for hc in hole_cards:
            if hc.rank == card.rank and hc.suit == card.suit:
                return True
    return False


def calc_fold_rate(p):
    stats = p['stats'] if isinstance(p, dict) else p.stats
    return stats['folds'] / stats['hands'] if stats['hands'] > 0 else 0


def avg_fold_rate(opponents):
    if not opponents:
        return 0
    return sum(calc_fold_rate(p) for p in opponents) / len(opponents)


def is_pocket_pair(hole):
    return Card(hole[0]).rank == Card(hole[1]).rank


def analyze_hand_context(hole, board):
    hand = Hand.solve(list(hole) + list(board))
    board_ranks = [Card(c).rank for c in board]
    highest_board = max(board_ranks) if board_ranks else 0
    pp = is_pocket_pair(hole)
    is_top = False
    is_over = False
    if hand.name == 'Pair':
        pair_rank = hand.cards[0].rank
        is_top = pair_rank == highest_board
        is_over = pp and pair_rank > highest_board
    return {'is_top_pair': is_top, 'is_over_pair': is_over}


def analyze_draw_potential(hole, board):
    all_cards = list(hole) + list(board)
    draws = {'flush_draw': False, 'straight_draw': False, 'outs': 0}
    suits = {}
    for c in all_cards:
        suit = c[1]
        suits[suit] = suits.get(suit, 0) + 1
    suit_counts = list(suits.values())
    has_flush = any(c >= 5 for c in suit_counts)
    if not has_flush:
        draws['flush_draw'] = any(c == 4 for c in suit_counts)
    flush_outs = 9 if draws['flush_draw'] else 0

    ranks = [Card(c).rank for c in all_cards]
    if 14 in ranks:
        ranks.append(1)
    unique = sorted(set(ranks))
    straights_list = [list(range(start, start + 5)) for start in range(1, 11)]
    straight_outs = 0
    has_straight = False
    missing_ranks = set()
    for seq in straights_list:
        missing = [r for r in seq if r not in unique]
        if len(missing) == 0:
            has_straight = True
            break
        if len(missing) == 1:
            draws['straight_draw'] = True
            mr = missing[0]
            missing_ranks.add(mr)
            if mr == seq[0] or mr == seq[4]:
                straight_outs = 8
    if has_straight:
        draws['straight_draw'] = False
        straight_outs = 0
    elif draws['straight_draw'] and straight_outs == 0:
        straight_outs = 8 if len(missing_ranks) >= 2 else 4
    draws['outs'] = flush_outs + straight_outs
    return draws


RANK_MAP = {
    '2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8,
    '9': 9, 'T': 10, 'J': 11, 'Q': 12, 'K': 13, 'A': 14,
}


def evaluate_board_texture(board):
    if not board or len(board) < 3:
        return 0
    ranks = []
    rank_counts = {}
    suit_counts = {}
    for card in board:
        r = card[0]
        s = card[1]
        ranks.append(RANK_MAP.get(r, 0))
        rank_counts[r] = rank_counts.get(r, 0) + 1
        suit_counts[s] = suit_counts.get(s, 0) + 1
    max_rank_count = max(rank_counts.values())
    pair_risk = (max_rank_count - 1) / (len(board) - 1) if max_rank_count > 1 else 0
    max_suit_count = max(suit_counts.values())
    suit_risk = (max_suit_count - 1) / (len(board) - 1)
    ranks_for_straight = ranks[:]
    if 14 in ranks_for_straight:
        ranks_for_straight.append(1)
    unique = sorted(set(ranks_for_straight))
    max_consecutive = 1
    current_run = 1
    for i in range(1, len(unique)):
        if unique[i] == unique[i - 1] + 1:
            current_run += 1
        else:
            current_run = 1
        if current_run > max_consecutive:
            max_consecutive = current_run
    connectedness = max(0, (max_consecutive - 2) / (len(board) - 2)) if max_consecutive >= 3 else 0
    texture_risk = (connectedness + suit_risk + pair_risk) / 3
    return max(0, min(1, texture_risk))


def preflop_hand_score(card_a, card_b):
    order = '23456789TJQKA'
    base = {
        'A': 10, 'K': 8, 'Q': 7, 'J': 6, 'T': 5,
        '9': 4.5, '8': 4, '7': 3.5, '6': 3, '5': 2.5,
        '4': 2, '3': 1.5, '2': 1,
    }
    r1, r2 = card_a[0], card_b[0]
    s1, s2 = card_a[1], card_b[1]
    i1, i2 = order.index(r1), order.index(r2)
    if i1 < i2:
        r1, r2 = r2, r1
        s1, s2 = s2, s1
        i1, i2 = i2, i1
    score = base[r1]
    if r1 == r2:
        score *= 2
        if score < 5:
            score = 5
    if s1 == s2:
        score += 2
    gap = i1 - i2 - 1
    if gap == 1:
        score -= 1
    elif gap == 2:
        score -= 2
    elif gap == 3:
        score -= 4
    elif gap >= 4:
        score -= 5
    if gap <= 1 and i1 < order.index('Q'):
        score += 1
    return max(0, min(10, score))


def find_next_active_player(players, start_idx):
    for i in range(1, len(players) + 1):
        idx = (start_idx + i) % len(players)
        if not players[idx]['folded']:
            return players[idx]
    return players[start_idx]


def compute_position_factor(players, active, player, current_phase_index):
    seat_idx = active.index(player)
    if current_phase_index == 0:
        bb_idx = next((i for i, p in enumerate(players) if p['big_blind']), 0)
        first_to_act = find_next_active_player(players, bb_idx)
    else:
        dealer_idx = next((i for i, p in enumerate(players) if p['dealer']), 0)
        first_to_act = find_next_active_player(players, dealer_idx)
    ref_idx = active.index(first_to_act)
    pos = (seat_idx - ref_idx + len(active)) % len(active)
    return pos / (len(active) - 1) if len(active) > 1 else 0


def evaluate_hand_strength(player, community_cards, preflop):
    if preflop:
        return {
            'strength': preflop_hand_score(player['cards'][0], player['cards'][1]),
            'solved_hand': None,
        }
    cards = [player['cards'][0], player['cards'][1]] + list(community_cards)
    solved_hand = Hand.solve(cards)
    return {
        'strength': solved_hand.rank + hand_tiebreaker(solved_hand),
        'solved_hand': solved_hand,
    }


def compute_postflop_context(player, community_cards, preflop):
    context = {
        'top_pair': False,
        'over_pair': False,
        'draw_chance': False,
        'draw_outs': 0,
        'draw_equity': 0,
        'texture_risk': 0,
    }
    if preflop or len(community_cards) < 3:
        return context
    hole = [player['cards'][0], player['cards'][1]]
    ctx = analyze_hand_context(hole, community_cards)
    context['top_pair'] = ctx['is_top_pair']
    context['over_pair'] = ctx['is_over_pair']
    draws = analyze_draw_potential(hole, community_cards)
    context['draw_chance'] = draws['flush_draw'] or draws['straight_draw']
    context['draw_outs'] = draws['outs']
    if context['draw_outs'] > 0:
        df = 0.04 if len(community_cards) == 3 else 0.02 if len(community_cards) == 4 else 0
        context['draw_equity'] = min(1, context['draw_outs'] * df)
    context['texture_risk'] = evaluate_board_texture(community_cards)
    return context


def get_m_zone(m_ratio):
    if m_ratio < M_RATIO_DEAD_MAX:
        return 'dead'
    if m_ratio <= M_RATIO_RED_MAX:
        return 'red'
    if m_ratio <= M_RATIO_ORANGE_MAX:
        return 'orange'
    if m_ratio <= M_RATIO_YELLOW_MAX:
        return 'yellow'
    return 'green'


def compute_commitment_metrics(need_to_call, player, spr, remaining_streets):
    projected_invested = player['total_bet'] + max(0, need_to_call)
    invested_ratio = projected_invested / max(1, projected_invested + player['chips'])
    call_cost_ratio = need_to_call / max(1, player['chips'])
    spr_pressure = max(0, min(1, (spr - COMMIT_SPR_MIN) / (COMMIT_SPR_MAX - COMMIT_SPR_MIN)))
    invest_pressure = max(0, min(1, (invested_ratio - COMMIT_INVEST_START) / (COMMIT_INVEST_END - COMMIT_INVEST_START)))
    call_pressure = max(0, min(1, call_cost_ratio / COMMIT_CALL_RATIO_REF))
    street_pressure = min(1, remaining_streets / 2)
    commitment_pressure = (invest_pressure * 0.6 + call_pressure * 0.4) * spr_pressure * street_pressure
    commitment_penalty = commitment_pressure * COMMITMENT_PENALTY_MAX
    return {'commitment_pressure': commitment_pressure, 'commitment_penalty': commitment_penalty}


def compute_elimination_risk(stack_ratio):
    risk = max(0, min(1, (stack_ratio - ELIMINATION_RISK_START) / (ELIMINATION_RISK_FULL - ELIMINATION_RISK_START)))
    elimination_penalty = risk * ELIMINATION_PENALTY_MAX
    return {'elimination_risk': risk, 'elimination_penalty': elimination_penalty}


def decide_harrington_action(params):
    m_zone = params['m_zone']
    strength_ratio = params['strength_ratio']
    facing_raise = params['facing_raise']
    needs_to_call = params['needs_to_call']
    need_to_call = params['need_to_call']
    player_chips = params['player_chips']
    can_shove = params['can_shove']
    can_raise = params['can_raise']
    dead_push_threshold = params['dead_push_threshold']
    red_push_threshold = params['red_push_threshold']
    orange_push_threshold = params['orange_push_threshold']
    yellow_raise_threshold = params['yellow_raise_threshold']
    yellow_shove_threshold = params['yellow_shove_threshold']
    red_call_threshold = params['red_call_threshold']
    orange_call_threshold = params['orange_call_threshold']
    yellow_call_threshold = params['yellow_call_threshold']
    yellow_raise_size = params['yellow_raise_size']

    if m_zone == 'dead':
        if facing_raise and needs_to_call:
            if strength_ratio >= dead_push_threshold:
                return {'action': 'raise', 'amount': player_chips} if can_shove else {'action': 'call', 'amount': min(player_chips, need_to_call)}
            return {'action': 'fold'}
        if can_shove and strength_ratio >= dead_push_threshold:
            return {'action': 'raise', 'amount': player_chips}
        return {'action': 'fold'} if needs_to_call else {'action': 'check'}
    elif m_zone == 'red':
        if facing_raise and needs_to_call:
            if strength_ratio >= red_call_threshold:
                return {'action': 'call', 'amount': min(player_chips, need_to_call)}
            return {'action': 'fold'}
        if can_shove and strength_ratio >= red_push_threshold:
            return {'action': 'raise', 'amount': player_chips}
        return {'action': 'fold'} if needs_to_call else {'action': 'check'}
    elif m_zone == 'orange':
        if facing_raise and needs_to_call:
            if strength_ratio >= orange_call_threshold:
                return {'action': 'call', 'amount': min(player_chips, need_to_call)}
            return {'action': 'fold'}
        if can_shove and strength_ratio >= orange_push_threshold:
            return {'action': 'raise', 'amount': player_chips}
        return {'action': 'fold'} if needs_to_call else {'action': 'check'}
    elif m_zone == 'yellow':
        if facing_raise and needs_to_call:
            if can_shove and strength_ratio >= yellow_shove_threshold:
                return {'action': 'raise', 'amount': player_chips}
            if strength_ratio >= yellow_call_threshold:
                return {'action': 'call', 'amount': min(player_chips, need_to_call)}
            return {'action': 'fold'}
        if can_shove and strength_ratio >= yellow_shove_threshold:
            return {'action': 'raise', 'amount': player_chips}
        if can_raise and strength_ratio >= yellow_raise_threshold:
            return {'action': 'raise', 'amount': yellow_raise_size()}
        return {'action': 'fold'} if needs_to_call else {'action': 'check'}
    return None


def choose_bot_action(player, ctx):
    current_bet = ctx['current_bet']
    pot = ctx['pot']
    small_blind = ctx['small_blind']
    big_blind = ctx['big_blind']
    raises_this_round = ctx['raises_this_round']
    current_phase_index = ctx['current_phase_index']
    players = ctx['players']
    last_raise = ctx['last_raise']
    community_cards = ctx['community_cards']

    need_to_call = current_bet - player['round_bet']
    needs_to_call = need_to_call > 0
    min_raise_amount = max(last_raise, need_to_call + last_raise)
    pot_odds = need_to_call / (pot + need_to_call) if (pot + need_to_call) > 0 else 0
    stack_ratio = need_to_call / player['chips'] if player['chips'] > 0 else 1
    spr = player['chips'] / max(1, pot + need_to_call)
    m_ratio = player['chips'] / max(1, small_blind + big_blind)
    facing_raise = current_bet > big_blind if current_phase_index == 0 else current_bet > 0
    can_raise = raises_this_round < MAX_RAISES_PER_ROUND and player['chips'] > big_blind
    can_shove = raises_this_round < MAX_RAISES_PER_ROUND

    active = [p for p in players if not p['folded']]
    opponents = [p for p in active if p['name'] != player['name']]
    active_opponents = len(opponents)
    opponent_stacks = [p['chips'] for p in opponents]
    max_opponent_stack = max(opponent_stacks) if opponent_stacks else 0
    effective_stack = min(player['chips'], max_opponent_stack) if opponent_stacks else player['chips']
    am_chipleader = player['chips'] > max_opponent_stack if opponent_stacks else True
    shortstack_relative = bool(opponent_stacks and effective_stack == player['chips'] and player['chips'] < max_opponent_stack * SHORTSTACK_RELATIVE)
    bot_line = player.get('bot_line', {})
    non_value_aggression_made = bot_line.get('non_value_aggression_made', False)

    position_factor = compute_position_factor(players, active, player, current_phase_index)
    preflop = len(community_cards) == 0
    result = evaluate_hand_strength(player, community_cards, preflop)
    strength = result['strength']
    solved_hand = result['solved_hand']
    hole_cards = [player['cards'][0], player['cards'][1]]

    hole_improves_hand = False
    if not preflop and solved_hand:
        if len(community_cards) < 5:
            hole_improves_hand = solved_hand_uses_hole_cards(hole_cards, solved_hand)
        elif len(community_cards) == 5:
            board_hand = Hand.solve(community_cards)
            hole_improves_hand = get_solved_hand_score(solved_hand) > get_solved_hand_score(board_hand)

    postflop_ctx = compute_postflop_context(player, community_cards, preflop)
    top_pair = postflop_ctx['top_pair']
    over_pair = postflop_ctx['over_pair']
    draw_chance = postflop_ctx['draw_chance']
    draw_outs = postflop_ctx['draw_outs']
    draw_equity = postflop_ctx['draw_equity']
    texture_risk = postflop_ctx['texture_risk']
    is_made_hand = not preflop and solved_hand and solved_hand.rank >= 2
    is_draw = draw_outs >= 8
    is_weak_draw = 0 < draw_outs < 8
    is_dead_hand = not preflop and not is_made_hand and not is_draw and not is_weak_draw

    strength_base = strength / 10
    strength_ratio = strength_base
    m_zone = get_m_zone(m_ratio)
    is_green_zone = m_zone == 'green'
    premium_hand = strength_ratio >= PREMIUM_PREFLOP_RATIO if preflop else strength_ratio >= PREMIUM_POSTFLOP_RATIO
    raise_agg_adj = -CHIP_LEADER_RAISE_DELTA if am_chipleader else 0
    call_tight_adj = -SHORTSTACK_CALL_DELTA if shortstack_relative and stack_ratio < ELIMINATION_RISK_START else 0

    dead_push_threshold = max(0, DEAD_PUSH_RATIO + raise_agg_adj)
    red_push_threshold = max(0, RED_PUSH_RATIO + raise_agg_adj)
    orange_push_threshold = max(0, ORANGE_PUSH_RATIO + raise_agg_adj)
    yellow_raise_threshold = max(0, YELLOW_RAISE_RATIO + raise_agg_adj)
    yellow_shove_threshold = max(0, YELLOW_SHOVE_RATIO + raise_agg_adj)
    red_call_threshold = min(1, RED_CALL_RATIO + call_tight_adj)
    orange_call_threshold = min(1, ORANGE_CALL_RATIO + call_tight_adj)
    yellow_call_threshold = min(1, YELLOW_CALL_RATIO + call_tight_adj)
    use_harrington = preflop and not is_green_zone

    remaining_streets = 3 if preflop else 2 if len(community_cards) == 3 else 1 if len(community_cards) == 4 else 0
    cm = compute_commitment_metrics(need_to_call, player, spr, remaining_streets)
    commitment_penalty = cm['commitment_penalty']
    er = compute_elimination_risk(stack_ratio) if needs_to_call else {'elimination_risk': 0, 'elimination_penalty': 0}
    elimination_penalty = er['elimination_penalty']

    risk_adjusted_red_call = min(1, red_call_threshold + elimination_penalty)
    risk_adjusted_orange_call = min(1, orange_call_threshold + elimination_penalty)
    risk_adjusted_yellow_call = min(1, yellow_call_threshold + elimination_penalty)

    call_barrier_base = min(1, max(0, pot_odds + call_tight_adj)) if preflop else min(1, max(0, POSTFLOP_CALL_BARRIER + call_tight_adj))
    call_barrier = min(1, call_barrier_base + commitment_penalty) if preflop else call_barrier_base

    if not preflop:
        call_barrier_adj = 0
        if hole_improves_hand:
            if over_pair:
                call_barrier_adj -= 0.03
            elif top_pair:
                call_barrier_adj -= 0.02
        if draw_outs >= 8:
            call_barrier_adj -= 0.02 if len(community_cards) == 3 else 0.01 if len(community_cards) == 4 else 0
        if active_opponents <= 1:
            call_barrier_adj -= 0.02
        if texture_risk > 0.6:
            call_barrier_adj += 0.02
        if spr < 3:
            call_barrier_adj -= 0.01
        elif spr > 6:
            call_barrier_adj += 0.01
        call_barrier_adj = max(-0.04, min(0.04, call_barrier_adj))

        street_index = 1 if len(community_cards) == 3 else 2 if len(community_cards) == 4 else 3 if len(community_cards) == 5 else 0
        raise_level_for_calls = raises_this_round if facing_raise and raises_this_round > 0 else 0
        street_pressure = street_index * 0.01 if needs_to_call else 0
        weak_draw_pressure = street_index * 0.01 if needs_to_call and is_weak_draw else 0
        dead_hand_pressure = street_index * 0.02 if needs_to_call and is_dead_hand else 0
        barrel_pressure = raise_level_for_calls * 0.02 if needs_to_call else 0
        pot_odds_adj = max(-0.12, min(0.08, (0.25 - pot_odds) * 0.6)) if needs_to_call else 0
        pot_odds_shift = -pot_odds_adj
        if needs_to_call and is_dead_hand:
            pot_odds_shift *= 0.35
        elif needs_to_call and is_weak_draw:
            pot_odds_shift *= 0.5
        commitment_shift = commitment_penalty * 0.8 if needs_to_call else 0
        call_barrier = call_barrier_base + call_barrier_adj + pot_odds_shift + commitment_shift
        call_barrier += street_pressure + weak_draw_pressure + dead_hand_pressure + barrel_pressure
        if needs_to_call and is_dead_hand:
            dead_hand_floor = 0.20 if street_index == 1 else 0.22 if street_index == 2 else 0.24
            call_barrier = max(call_barrier, dead_hand_floor)
        if needs_to_call and is_weak_draw:
            if street_index >= 2:
                call_barrier = 1
            elif street_index == 1 and (pot_odds > 0.18 or raise_level_for_calls > 0):
                call_barrier = 1
        call_barrier = min(1, max(0.10, call_barrier))

    elimination_barrier = min(1, call_barrier + elimination_penalty) if needs_to_call else call_barrier

    opp_agg_adj = (OPPONENT_THRESHOLD - active_opponents) * AGG_FACTOR if active_opponents < OPPONENT_THRESHOLD else 0
    threshold_adj = (OPPONENT_THRESHOLD - active_opponents) * THRESHOLD_FACTOR if active_opponents < OPPONENT_THRESHOLD else 0
    base_aggressiveness = 0.8 + 0.4 * position_factor if preflop else 1 + 0.6 * position_factor
    aggressiveness = base_aggressiveness + opp_agg_adj
    raise_threshold = 8 - 2 * position_factor if preflop else 2.6 - 0.8 * position_factor
    raise_threshold = max(1, raise_threshold - (threshold_adj if preflop else 0))
    if am_chipleader:
        raise_threshold = max(1, raise_threshold - CHIP_LEADER_RAISE_DELTA * 10)
    decision_strength = strength if preflop else strength_ratio * 10

    bluff_chance = 0
    fold_rate = 0
    stats_weight = 0
    avg_vpip = 0
    avg_agg = 0

    stat_opponents = [p for p in players if p['name'] != player['name']]
    if stat_opponents:
        avg_vpip = sum((p['stats']['vpip'] + 1) / (p['stats']['hands'] + 2) for p in stat_opponents) / len(stat_opponents)
        avg_agg = sum((p['stats']['aggressive_acts'] + 1) / (p['stats']['calls'] + 1) for p in stat_opponents) / len(stat_opponents)
        fold_rate = avg_fold_rate(stat_opponents)
        avg_hands = sum(p['stats']['hands'] for p in stat_opponents) / len(stat_opponents)
        weight = 0 if avg_hands < MIN_HANDS_FOR_WEIGHT else 1 - math.exp(-(avg_hands - MIN_HANDS_FOR_WEIGHT) / WEIGHT_GROWTH)
        stats_weight = weight
        bluff_chance = min(0.3, fold_rate) * weight
        bluff_chance *= 1 - texture_risk * 0.5
        bluff_agg_factor = max(0.8, min(1.2, aggressiveness))
        bluff_chance = min(0.3, bluff_chance * bluff_agg_factor)
        if avg_vpip < 0.25:
            raise_threshold -= 0.5 * weight
            aggressiveness += 0.1 * weight
        elif avg_vpip > 0.5:
            raise_threshold += 0.5 * weight
            aggressiveness -= 0.1 * weight
        if avg_agg > 1.5:
            aggressiveness -= 0.1 * weight
        elif avg_agg < 0.7:
            aggressiveness += 0.1 * weight

    raise_threshold = max(1, raise_threshold - (aggressiveness - 1) * 0.8)
    if not preflop:
        raise_adj = 0
        if hole_improves_hand:
            if over_pair:
                raise_adj -= 0.35
            elif top_pair:
                raise_adj -= 0.2
        if draw_outs >= 8:
            raise_adj -= 0.15 if len(community_cards) == 3 else 0.08 if len(community_cards) == 4 else 0
        if active_opponents <= 1:
            raise_adj -= 0.15
        if texture_risk > 0.6:
            raise_adj += 0.15
        if spr < 3:
            raise_adj -= 0.1
        elif spr > 6:
            raise_adj += 0.1
        raise_adj = max(-0.5, min(0.5, raise_adj))
        raise_threshold += raise_adj
        raise_threshold = max(1.4, raise_threshold)

    raise_level = max(0, raises_this_round) if facing_raise and raises_this_round > 0 else 0
    raise_threshold += raise_level * RERAISE_RATIO_STEP * 10
    bet_agg_factor = max(0.9, min(1.1, aggressiveness))
    shove_agg_adj = max(-0.08, min(0.08, (aggressiveness - 1) * 0.12))

    line_abort = False
    if not preflop and bot_line and bot_line.get('preflop_aggressor', False):
        line_abort = texture_risk > 0.7 and strength_ratio < 0.45 and draw_equity == 0

    def cap_green_non_premium(amount):
        if not is_green_zone or premium_hand:
            return amount
        cap_ratio = 0.3 if spr < 3 else 0.2 if spr > 6 else GREEN_MAX_STACK_BET
        cap = int(player['chips'] * cap_ratio)
        return min(amount, cap)

    def value_bet_size():
        base_val = 0.55
        if preflop:
            if strength_ratio >= 0.9:
                base_val += 0.15
            base_val += active_opponents * 0.04
            base_val += (1 - position_factor) * 0.05
            if position_factor < 0.3 and strength_ratio >= 0.8:
                base_val += 0.1
        else:
            base_val = 0.7 if texture_risk > 0.6 else 0.6 if texture_risk > 0.3 else 0.45
            if strength_ratio > 0.95:
                base_val += 0.1
            base_val += active_opponents * 0.03
            base_val += (1 - position_factor) * 0.05
        if spr < 2:
            base_val += 0.1
        elif spr < 4:
            base_val += 0.05
        elif spr > 6:
            base_val -= 0.05
        rand_v = random.uniform(-0.1, 0.1)
        factor = min(1, max(0.35, base_val + rand_v))
        sized = round_to_10(min(player['chips'], (pot + need_to_call) * factor * bet_agg_factor))
        return cap_green_non_premium(sized)

    def bluff_bet_size():
        base_val = 0.25 + texture_risk * 0.05
        base_val += active_opponents * 0.02
        base_val += (1 - position_factor) * 0.03
        if spr < 3:
            base_val += 0.05
        elif spr > 5:
            base_val -= 0.05
        rand_v = random.uniform(-0.04, 0.04)
        factor = min(0.45, max(0.2, base_val + rand_v))
        sized = round_to_10(min(player['chips'], (pot + need_to_call) * factor * bet_agg_factor))
        return cap_green_non_premium(sized)

    def protection_bet_size():
        base_val = 0.45 + texture_risk * 0.25
        base_val += active_opponents * 0.03
        base_val += (1 - position_factor) * 0.04
        if spr < 3:
            base_val += 0.1
        elif spr > 5:
            base_val -= 0.05
        rand_v = random.uniform(-0.05, 0.05)
        factor = min(0.8, max(0.35, base_val + rand_v))
        sized = round_to_10(min(player['chips'], (pot + need_to_call) * factor * bet_agg_factor))
        return cap_green_non_premium(sized)

    def yellow_raise_size():
        base_val = big_blind * (2.5 + random.uniform(0, 0.5))
        sized = round_to_10(base_val * bet_agg_factor)
        return min(player['chips'], max(min_raise_amount, sized))

    decision = None
    if use_harrington:
        decision = decide_harrington_action({
            'm_zone': m_zone,
            'facing_raise': facing_raise,
            'needs_to_call': needs_to_call,
            'strength_ratio': strength_ratio,
            'dead_push_threshold': dead_push_threshold,
            'red_push_threshold': red_push_threshold,
            'orange_push_threshold': orange_push_threshold,
            'yellow_raise_threshold': yellow_raise_threshold,
            'yellow_shove_threshold': yellow_shove_threshold,
            'red_call_threshold': risk_adjusted_red_call,
            'orange_call_threshold': risk_adjusted_orange_call,
            'yellow_call_threshold': risk_adjusted_yellow_call,
            'can_shove': can_shove,
            'can_raise': can_raise,
            'need_to_call': need_to_call,
            'player_chips': player['chips'],
            'yellow_raise_size': yellow_raise_size,
        })

    if not decision:
        shallow_shove = max(0, min(1, 0.65 - shove_agg_adj))
        shortstack_shove = max(0, min(1, 0.75 - shove_agg_adj))
        if spr <= 1.2 and strength_ratio >= shallow_shove:
            decision = {'action': 'raise', 'amount': player['chips']}
        elif preflop and player['chips'] <= big_blind * 10 and strength_ratio >= shortstack_shove:
            decision = {'action': 'raise', 'amount': player['chips']}

    if not decision:
        if need_to_call <= 0:
            if can_raise and decision_strength >= raise_threshold:
                raise_amt = value_bet_size()
                raise_amt = max(min_raise_amount, raise_amt)
                if abs(decision_strength - raise_threshold) <= STRENGTH_TIE_DELTA:
                    decision = {'action': 'check'} if random.random() < 0.5 else {'action': 'raise', 'amount': raise_amt}
                else:
                    decision = {'action': 'raise', 'amount': raise_amt}
            else:
                decision = {'action': 'check'}
        elif can_raise and decision_strength >= raise_threshold and stack_ratio <= 1 / 3:
            raise_amt = protection_bet_size()
            raise_amt = max(min_raise_amount, raise_amt)
            if abs(decision_strength - raise_threshold) <= STRENGTH_TIE_DELTA:
                alt = {'action': 'call', 'amount': min(player['chips'], need_to_call)} if strength_ratio >= elimination_barrier and stack_ratio <= (0.5 if preflop else 0.7) else {'action': 'fold'}
                decision = {'action': 'raise', 'amount': raise_amt} if random.random() < 0.5 else alt
            else:
                decision = {'action': 'raise', 'amount': raise_amt}
        elif strength_ratio >= elimination_barrier and stack_ratio <= (0.5 if preflop else 0.7):
            call_amt = min(player['chips'], need_to_call)
            if abs(strength_ratio - elimination_barrier) <= ODDS_TIE_DELTA:
                decision = {'action': 'call', 'amount': call_amt} if random.random() < 0.5 else {'action': 'fold'}
            else:
                decision = {'action': 'call', 'amount': call_amt}
        else:
            decision = {'action': 'fold'}

    is_bluff = False
    is_stab = False
    if not use_harrington:
        facing_all_in = any(p.get('all_in', False) for p in opponents)
        if decision['action'] == 'fold' and facing_all_in:
            good_threshold = ALLIN_HAND_PREFLOP if preflop else ALLIN_HAND_POSTFLOP
            risk_adjusted_threshold = min(1, good_threshold + elimination_penalty)
            if strength_ratio >= risk_adjusted_threshold:
                decision = {'action': 'call', 'amount': min(player['chips'], need_to_call)}

        if bluff_chance > 0 and can_raise and not facing_raise and (not preflop or strength_ratio >= MIN_PREFLOP_BLUFF_RATIO) and decision['action'] in ('check', 'fold') and not facing_all_in and not non_value_aggression_made:
            if random.random() < bluff_chance:
                bluff_amt = max(min_raise_amount, bluff_bet_size())
                decision = {'action': 'raise', 'amount': bluff_amt}
                is_bluff = True

        if not preflop and current_bet == 0 and decision['action'] == 'check' and can_raise and not facing_raise and bot_line and bot_line.get('preflop_aggressor', False) and not line_abort and strength_ratio < 0.9:
            if current_phase_index == 1 and bot_line.get('cbet_intent', False):
                wants_bluff = strength_ratio < 0.6 and draw_equity == 0
                if not wants_bluff or not non_value_aggression_made:
                    bet = protection_bet_size() if strength_ratio >= 0.6 or draw_equity > 0 else bluff_bet_size()
                    decision = {'action': 'raise', 'amount': min(player['chips'], max(last_raise, bet))}
                    if wants_bluff:
                        is_bluff = True
            elif current_phase_index == 2 and bot_line.get('barrel_intent', False):
                wants_bluff = strength_ratio < 0.6 and draw_equity == 0
                if not wants_bluff or not non_value_aggression_made:
                    bet = protection_bet_size() if strength_ratio >= 0.65 or draw_equity > 0 else bluff_bet_size()
                    decision = {'action': 'raise', 'amount': min(player['chips'], max(last_raise, bet))}
                    if wants_bluff:
                        is_bluff = True

        if not preflop and decision['action'] == 'raise' and strength_ratio >= 0.95 and spr <= 2 and random.random() < 0.3:
            decision['amount'] = max(decision['amount'], protection_bet_size() * 2)

        if not preflop and not needs_to_call and strength_ratio >= 0.9 and decision['action'] == 'raise' and random.random() < 0.3:
            decision = {'action': 'check'}

        if not preflop and current_bet == 0 and decision['action'] == 'check' and can_raise and not facing_raise and texture_risk < 0.4 and (fold_rate > 0.25 or draw_equity > 0) and random.random() < max(0.05, min(0.35, 0.05 + position_factor * 0.3)) and not non_value_aggression_made:
            bet_amt = protection_bet_size()
            decision = {'action': 'raise', 'amount': max(last_raise, bet_amt)}
            is_stab = True

    reraise_value_ratio = RERAISE_TOP_PAIR_RATIO if (top_pair or over_pair) else RERAISE_VALUE_RATIO
    if decision['action'] == 'raise' and raise_level > 0 and strength_ratio < reraise_value_ratio:
        decision = {'action': 'call', 'amount': min(player['chips'], need_to_call)} if need_to_call > 0 else {'action': 'check'}
        is_bluff = False
        is_stab = False

    if decision['action'] == 'raise':
        min_raise = need_to_call + last_raise
        if decision.get('amount', 0) < min_raise and decision.get('amount', 0) < player['chips']:
            decision = {'action': 'call', 'amount': min(player['chips'], need_to_call)} if need_to_call > 0 else {'action': 'check'}

    return decision
