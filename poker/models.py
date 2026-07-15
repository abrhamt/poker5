import json
from django.db import models


class Game(models.Model):
    table_id = models.CharField(max_length=16, unique=True, db_index=True)
    phase = models.CharField(max_length=16, default='preflop')
    pot = models.IntegerField(default=0)
    current_bet = models.IntegerField(default=0)
    last_raise = models.IntegerField(default=20)
    small_blind = models.IntegerField(default=10)
    big_blind = models.IntegerField(default=20)
    raises_this_round = models.IntegerField(default=0)
    dealer_orbit_count = models.IntegerField(default=-1)
    game_started = models.BooleanField(default=False)
    game_finished = models.BooleanField(default=False)
    open_cards_mode = models.BooleanField(default=False)
    spectator_mode = models.BooleanField(default=False)
    initial_dealer_name = models.CharField(max_length=64, null=True, blank=True)
    deck = models.TextField(default='[]')
    card_graveyard = models.TextField(default='[]')
    community_cards = models.TextField(default='[]')
    current_player_index = models.IntegerField(default=0)
    total_hands = models.IntegerField(default=0)
    created_at = models.DateTimeField(auto_now_add=True)
    updated_at = models.DateTimeField(auto_now=True)
    version = models.IntegerField(default=0)
    notifications = models.TextField(default='[]')

    class Meta:
        db_table = 'poker_game'


class Player(models.Model):
    game = models.ForeignKey(Game, related_name='players', on_delete=models.CASCADE)
    name = models.CharField(max_length=64)
    seat_index = models.IntegerField()
    is_bot = models.BooleanField(default=False)
    chips = models.IntegerField(default=2000)
    round_bet = models.IntegerField(default=0)
    total_bet = models.IntegerField(default=0)
    folded = models.BooleanField(default=False)
    all_in = models.BooleanField(default=False)
    is_dealer = models.BooleanField(default=False)
    is_small_blind = models.BooleanField(default=False)
    is_big_blind = models.BooleanField(default=False)
    card1 = models.CharField(max_length=4, null=True, blank=True)
    card2 = models.CharField(max_length=4, null=True, blank=True)
    win_probability = models.FloatField(null=True, blank=True)
    stats_data = models.TextField(default='{}')

    class Meta:
        db_table = 'poker_player'
        unique_together = [('game', 'seat_index')]
