"""
Poker hand evaluator — direct port of pokersolver.js v2.1.2
Copyright (c) 2016, James Simpson of GoldFire Studios
"""

RANK_ORDER = ['1', '2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A']


class Card:
    def __init__(self, code):
        self.value = code[0].upper()
        self.suit = code[1].lower()
        self.rank = RANK_ORDER.index(self.value)
        self.wild_value = self.value

    def __repr__(self):
        return self.wild_value.replace('T', '10') + self.suit

    @staticmethod
    def sort_key(card):
        return -card.rank

    def __eq__(self, other):
        return self.rank == other.rank and self.suit == other.suit

    def __hash__(self):
        return hash((self.rank, self.suit))


GAME_RULES = {
    'standard': {
        'cardsInHand': 5,
        'handValues': None,
        'wildValue': None,
        'wildStatus': 1,
        'wheelStatus': 0,
        'sfQualify': 5,
        'lowestQualified': None,
        'noKickers': False,
    },
}


class Game:
    def __init__(self, descr='standard'):
        self.descr = descr
        rules = GAME_RULES.get(descr, GAME_RULES['standard'])
        self.cards_in_hand = rules['cardsInHand']
        self.wild_value = rules['wildValue']
        self.wild_status = rules['wildStatus']
        self.wheel_status = rules['wheelStatus']
        self.sf_qualify = rules['sfQualify']
        self.lowest_qualified = rules['lowestQualified']
        self.no_kickers = rules['noKickers']


class Hand:
    def __init__(self, cards, name, game, can_disqualify=False):
        self.card_pool = []
        self.cards = []
        self.suits = {}
        self.values = {}
        self.wilds = []
        self.name = name
        self.game = game
        self.sf_length = 0
        self.always_qualifies = True
        self.rank = 0

        if can_disqualify and self.game.lowest_qualified:
            self.always_qualifies = False

        if game.descr == 'standard' and len(set(cards)) != len(cards):
            raise ValueError('Duplicate cards')

        hand_rank = len(self.game.hand_values)
        for i, hv in enumerate(self.game.hand_values):
            if hv is self.__class__:
                break
        self.rank = hand_rank - i

        self.card_pool = [Card(c) if isinstance(c, str) else c for c in cards]

        for card in self.card_pool:
            if card.value == self.game.wild_value:
                card.rank = -1
        self.card_pool.sort(key=Card.sort_key)

        for card in self.card_pool:
            if card.rank == -1:
                self.wilds.append(card)
            else:
                self.suits.setdefault(card.suit, []).append(card)
                self.values.setdefault(card.rank, []).append(card)

        sorted_ranks = sorted(self.values.keys(), reverse=True)
        self.values = {k: self.values[k] for k in sorted_ranks}
        self.is_possible = self.solve()

    def compare(self, other):
        if self.rank < other.rank:
            return 1
        elif self.rank > other.rank:
            return -1
        for i in range(5):
            if i < len(self.cards) and i < len(other.cards):
                if self.cards[i].rank < other.cards[i].rank:
                    return 1
                elif self.cards[i].rank > other.cards[i].rank:
                    return -1
        return 0

    def lose_to(self, other):
        return self.compare(other) > 0

    def get_num_cards_by_rank(self, val):
        cards = self.values.get(val, [])
        count = len(cards)
        for wild in self.wilds:
            if wild.rank > -1:
                continue
            if cards:
                if self.game.wild_status == 1 or cards[0].rank == len(RANK_ORDER) - 1:
                    count += 1
            elif self.game.wild_status == 1 or val == len(RANK_ORDER) - 1:
                count += 1
        return count

    def get_cards_for_flush(self, suit, set_ranks=False):
        cards = sorted(self.suits.get(suit, []), key=Card.sort_key)
        for wild in self.wilds:
            if set_ranks:
                j = 0
                while j < len(RANK_ORDER) and j < len(cards):
                    if cards[j].rank == len(RANK_ORDER) - 1 - j:
                        j += 1
                    else:
                        break
                wild.rank = len(RANK_ORDER) - 1 - j
                wild.wild_value = RANK_ORDER[wild.rank]
            cards.append(wild)
            cards.sort(key=Card.sort_key)
        return cards

    def reset_wild_cards(self):
        for wild in self.wilds:
            wild.rank = -1
            wild.wild_value = wild.value

    def next_highest(self):
        excluding = set(self.cards)
        picks = [c for c in self.card_pool if c not in excluding]
        if self.game.wild_status == 0:
            for card in picks:
                if card.rank == -1:
                    card.wild_value = 'A'
                    card.rank = len(RANK_ORDER) - 1
            picks.sort(key=Card.sort_key)
        return picks

    def qualifies_high(self):
        if not self.game.lowest_qualified or self.always_qualifies:
            return True
        q_hand = Hand.solve(self.game.lowest_qualified, self.game)
        return self.compare(q_hand) <= 0

    @staticmethod
    def winners(hands):
        hands = [h for h in hands if h.qualifies_high()]
        if not hands:
            return []
        highest_rank = max(h.rank for h in hands)
        hands = [h for h in hands if h.rank == highest_rank]
        result = []
        for h in hands:
            if not any(h.lose_to(other) for other in hands):
                result.append(h)
        return result

    @staticmethod
    def solve(cards, game=None, can_disqualify=False):
        if game is None:
            game = Game('standard')
        elif isinstance(game, str):
            game = Game(game)
        cards = cards or ['']
        game.hand_values = _hand_rankings(game.descr)
        result = None
        for hv in game.hand_values:
            result = hv(cards, game, can_disqualify)
            if result.is_possible:
                break
        return result


class StraightFlush(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Straight Flush', game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        possible_straight = None
        non_cards = []
        for suit in self.suits:
            cards = self.get_cards_for_flush(suit, False)
            if cards and len(cards) >= self.game.sf_qualify:
                possible_straight = cards
                break
        if possible_straight:
            if self.game.descr != 'standard':
                for suit in self.suits:
                    if possible_straight[0].suit != suit:
                        non_cards.extend(self.suits.get(suit, []))
                        _, non_wilds = Hand.strip_wilds(non_cards, self.game)
                        non_cards = non_wilds
            straight = Straight(possible_straight, self.game)
            if straight.is_possible:
                self.cards = straight.cards[:]
                self.cards.extend(non_cards)
                self.sf_length = straight.sf_length
        if self.cards and self.cards[0].rank == 13:
            self.descr = 'Royal Flush'
        elif len(self.cards) >= self.game.sf_qualify:
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + f"{suit} High"
        return len(self.cards) >= self.game.sf_qualify


class RoyalFlush(StraightFlush):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        result = super().solve()
        return result and getattr(self, 'descr', '') == 'Royal Flush'


class NaturalRoyalFlush(RoyalFlush):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, game, can_disqualify)


class WildRoyalFlush(RoyalFlush):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, game, can_disqualify)


class FiveOfAKind(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Five of a Kind', game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        for rank in self.values:
            if self.get_num_cards_by_rank(rank) == 5:
                self.cards = self.values.get(rank, [])[:]
                for wild in self.wilds:
                    if len(self.cards) >= 5:
                        break
                    if self.cards:
                        wild.rank = self.cards[0].rank
                    else:
                        wild.rank = len(RANK_ORDER) - 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
                self.cards.extend(self.next_highest()[:self.game.cards_in_hand - 5])
                break
        if len(self.cards) >= 5:
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + "'s"
        return len(self.cards) >= 5


class FourOfAKind(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Four of a Kind', game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        for rank in self.values:
            if self.get_num_cards_by_rank(rank) == 4:
                self.cards = self.values.get(rank, [])[:]
                for wild in self.wilds:
                    if len(self.cards) >= 4:
                        break
                    if self.cards:
                        wild.rank = self.cards[0].rank
                    else:
                        wild.rank = len(RANK_ORDER) - 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
                self.cards.extend(self.next_highest()[:self.game.cards_in_hand - 4])
                break
        if len(self.cards) >= 4:
            if self.game.no_kickers:
                self.cards = self.cards[:4]
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + "'s"
        return len(self.cards) >= 4


class FullHouse(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Full House', game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        for rank in self.values:
            if self.get_num_cards_by_rank(rank) == 3:
                self.cards = self.values.get(rank, [])[:]
                for wild in self.wilds:
                    if len(self.cards) >= 3:
                        break
                    if self.cards:
                        wild.rank = self.cards[0].rank
                    else:
                        wild.rank = len(RANK_ORDER) - 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
                break
        if len(self.cards) == 3:
            for rank in self.values:
                cards = self.values.get(rank, [])
                if cards and self.cards[0].wild_value == cards[0].wild_value:
                    continue
                if self.get_num_cards_by_rank(rank) >= 2:
                    self.cards.extend(cards or [])
                    for wild in self.wilds:
                        if wild.rank != -1:
                            continue
                        if cards:
                            wild.rank = cards[0].rank
                        elif self.cards[0].rank == len(RANK_ORDER) - 1 and self.game.wild_status == 1:
                            wild.rank = len(RANK_ORDER) - 2
                        else:
                            wild.rank = len(RANK_ORDER) - 1
                        wild.wild_value = RANK_ORDER[wild.rank]
                        self.cards.append(wild)
                    self.cards.extend(self.next_highest()[:self.game.cards_in_hand - 5])
                    break
        if len(self.cards) >= 5:
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + "'s over " + f"{self.cards[3]!s}"[:-1] + "'s"
        return len(self.cards) >= 5


class Flush(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Flush', game, can_disqualify)

    def solve(self):
        self.sf_length = 0
        self.reset_wild_cards()
        for suit in self.suits:
            cards = self.get_cards_for_flush(suit, True)
            if len(cards) >= self.game.sf_qualify:
                self.cards = cards
                break
        if len(self.cards) >= self.game.sf_qualify:
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + f"{suit} High"
            self.sf_length = len(self.cards)
            if len(self.cards) < self.game.cards_in_hand:
                self.cards.extend(self.next_highest()[:self.game.cards_in_hand - len(self.cards)])
        return len(self.cards) >= self.game.sf_qualify


class Straight(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Straight', game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        if self.game.wheel_status == 1:
            wheel = self.get_wheel()
            if wheel:
                wild_count = sum(1 for c in wheel if c.value == self.game.wild_value)
                for card in wheel:
                    if card.rank == 0:
                        card.rank = RANK_ORDER.index('A')
                        card.wild_value = 'A'
                        if card.value == '1':
                            card.value = 'A'
                wheel.sort(key=Card.sort_key)
                for _ in range(wild_count, len(self.wilds)):
                    if len(wheel) >= self.game.cards_in_hand:
                        break
                    card = self.wilds[wild_count]
                    card.rank = RANK_ORDER.index('A')
                    card.wild_value = 'A'
                    wheel.append(card)
                self.descr = f"{self.name}, Wheel"
                self.sf_length = self.game.sf_qualify
                if wheel[0].value == 'A':
                    wheel.extend(self.next_highest()[1:self.game.cards_in_hand - len(wheel) + 1])
                else:
                    wheel.extend(self.next_highest()[:self.game.cards_in_hand - len(wheel)])
                self.cards = wheel
                self.sf_length = self.game.sf_qualify
                return True
            self.reset_wild_cards()

        self.cards = self.get_gaps()
        for wild in self.wilds:
            check_cards = self.get_gaps(len(self.cards))
            if len(self.cards) == len(check_cards):
                if self.cards[0].rank < len(RANK_ORDER) - 1:
                    wild.rank = self.cards[0].rank + 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
                else:
                    wild.rank = self.cards[-1].rank - 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
            else:
                for j in range(1, len(self.cards)):
                    if self.cards[j - 1].rank - self.cards[j].rank > 1:
                        wild.rank = self.cards[j - 1].rank - 1
                        wild.wild_value = RANK_ORDER[wild.rank]
                        self.cards.append(wild)
                        break
            self.cards.sort(key=Card.sort_key)

        if len(self.cards) >= self.game.sf_qualify:
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + " High"
            self.cards = self.cards[:self.game.cards_in_hand]
            self.sf_length = len(self.cards)
            if len(self.cards) < self.game.cards_in_hand:
                if self.cards[self.sf_length - 1].rank == 0:
                    self.cards.extend(self.next_highest()[1:self.game.cards_in_hand - len(self.cards) + 1])
                else:
                    self.cards.extend(self.next_highest()[:self.game.cards_in_hand - len(self.cards)])
        return len(self.cards) >= self.game.sf_qualify

    def get_gaps(self, check_hand_length=None):
        wild_cards, cards_to_check = Hand.strip_wilds(self.card_pool, self.game)
        for card in cards_to_check[:]:
            if card.wild_value == 'A':
                cards_to_check.append(Card('1' + card.suit))
        cards_to_check.sort(key=Card.sort_key)

        if check_hand_length is None:
            check_hand_length = self.game.sf_qualify
            i = len(RANK_ORDER)
        else:
            i = cards_to_check[0].rank + 1

        gap_cards = []
        while i > 0:
            cards_list = []
            gap_count = 0
            for card in cards_to_check:
                if card.rank > i:
                    continue
                prev_card = cards_list[-1] if cards_list else None
                if prev_card is None:
                    diff = i - card.rank
                else:
                    diff = prev_card.rank - card.rank
                if diff is None:
                    cards_list.append(card)
                elif check_hand_length < (gap_count + diff + len(cards_list)):
                    break
                elif diff > 0:
                    cards_list.append(card)
                    gap_count += (diff - 1)
            if len(cards_list) > len(gap_cards):
                gap_cards = cards_list[:]
            if self.game.sf_qualify - len(gap_cards) <= len(wild_cards):
                break
            i -= 1
        return gap_cards

    def get_wheel(self):
        wild_cards, cards_to_check = Hand.strip_wilds(self.card_pool, self.game)
        for card in cards_to_check[:]:
            if card.wild_value == 'A':
                cards_to_check.append(Card('1' + card.suit))
        cards_to_check.sort(key=Card.sort_key)

        wheel_cards = []
        wild_count = 0
        for i in range(self.game.sf_qualify - 1, -1, -1):
            found = False
            for card in cards_to_check:
                if card.rank > i:
                    continue
                if card.rank < i:
                    break
                wheel_cards.append(card)
                found = True
                break
            if not found:
                if wild_count < len(wild_cards):
                    wild_cards[wild_count].rank = i
                    wild_cards[wild_count].wild_value = RANK_ORDER[i]
                    wheel_cards.append(wild_cards[wild_count])
                    wild_count += 1
                else:
                    return []
        return wheel_cards


class ThreeOfAKind(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Three of a Kind', game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        for rank in self.values:
            if self.get_num_cards_by_rank(rank) == 3:
                self.cards = self.values.get(rank, [])[:]
                for wild in self.wilds:
                    if len(self.cards) >= 3:
                        break
                    if self.cards:
                        wild.rank = self.cards[0].rank
                    else:
                        wild.rank = len(RANK_ORDER) - 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
                self.cards.extend(self.next_highest()[:self.game.cards_in_hand - 3])
                break
        if len(self.cards) >= 3:
            if self.game.no_kickers:
                self.cards = self.cards[:3]
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + "'s"
        return len(self.cards) >= 3


class TwoPair(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Two Pair', game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        for rank in self.values:
            cards = self.values.get(rank, [])
            if len(self.cards) > 0 and self.get_num_cards_by_rank(rank) == 2:
                self.cards.extend(cards or [])
                for wild in self.wilds:
                    if wild.rank != -1:
                        continue
                    if cards:
                        wild.rank = cards[0].rank
                    elif self.cards[0].rank == len(RANK_ORDER) - 1 and self.game.wild_status == 1:
                        wild.rank = len(RANK_ORDER) - 2
                    else:
                        wild.rank = len(RANK_ORDER) - 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
                self.cards.extend(self.next_highest()[:self.game.cards_in_hand - 4])
                break
            elif self.get_num_cards_by_rank(rank) == 2:
                self.cards.extend(cards or [])
                for wild in self.wilds:
                    if wild.rank != -1:
                        continue
                    if cards:
                        wild.rank = cards[0].rank
                    elif self.cards[0].rank == len(RANK_ORDER) - 1 and self.game.wild_status == 1:
                        wild.rank = len(RANK_ORDER) - 2
                    else:
                        wild.rank = len(RANK_ORDER) - 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
        if len(self.cards) >= 4:
            if self.game.no_kickers:
                self.cards = self.cards[:4]
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + "'s & " + f"{self.cards[2]!s}"[:-1] + "'s"
        return len(self.cards) >= 4


class OnePair(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'Pair', game, can_disqualify)

    def solve(self):
        self.reset_wild_cards()
        for rank in self.values:
            if self.get_num_cards_by_rank(rank) == 2:
                self.cards.extend(self.values.get(rank, []) or [])
                for wild in self.wilds:
                    if len(self.cards) >= 2:
                        break
                    if self.cards:
                        wild.rank = self.cards[0].rank
                    else:
                        wild.rank = len(RANK_ORDER) - 1
                    wild.wild_value = RANK_ORDER[wild.rank]
                    self.cards.append(wild)
                self.cards.extend(self.next_highest()[:self.game.cards_in_hand - 2])
                break
        if len(self.cards) >= 2:
            if self.game.no_kickers:
                self.cards = self.cards[:2]
            self.descr = f"{self.name}, {self.cards[0]!s}"[:-1] + "'s"
        return len(self.cards) >= 2


class HighCard(Hand):
    def __init__(self, cards, game, can_disqualify=False):
        super().__init__(cards, 'High Card', game, can_disqualify)

    def solve(self):
        self.cards = self.card_pool[:self.game.cards_in_hand]
        for card in self.cards:
            if card.value == self.game.wild_value:
                card.wild_value = 'A'
                card.rank = RANK_ORDER.index('A')
        if self.game.no_kickers:
            self.cards = self.cards[:1]
        self.cards.sort(key=Card.sort_key)
        self.descr = f"{self.cards[0]!s}"[:-1] + " High"
        return True


def _hand_rankings(game_descr):
    return [
        StraightFlush,
        FourOfAKind,
        FullHouse,
        Flush,
        Straight,
        ThreeOfAKind,
        TwoPair,
        OnePair,
        HighCard,
    ]


@staticmethod
def _strip_wilds(cards, game):
    wilds = []
    non_wilds = []
    for c in cards:
        card = c if isinstance(c, Card) else Card(c)
        if card.rank == -1:
            wilds.append(card)
        else:
            non_wilds.append(card)
    return wilds, non_wilds

Hand.strip_wilds = _strip_wilds
