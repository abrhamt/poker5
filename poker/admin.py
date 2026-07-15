from django.contrib import admin
from .models import Game, Player


@admin.register(Game)
class GameAdmin(admin.ModelAdmin):
    list_display = ('table_id', 'phase', 'pot', 'game_started', 'game_finished', 'version', 'created_at')
    list_filter = ('game_started', 'game_finished', 'phase')
    search_fields = ('table_id',)


@admin.register(Player)
class PlayerAdmin(admin.ModelAdmin):
    list_display = ('name', 'game', 'seat_index', 'is_bot', 'chips', 'folded', 'all_in')
    list_filter = ('is_bot', 'folded', 'all_in')
    search_fields = ('name', 'game__table_id')
