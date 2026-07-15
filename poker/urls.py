from django.urls import path
from . import views

urlpatterns = [
    path('', views.index, name='index'),
    path('hole-cards/', views.hole_cards, name='hole_cards'),
    path('api/start/', views.api_start_game, name='api_start'),
    path('api/act/', views.api_act, name='api_act'),
    path('api/state/', views.api_state, name='api_state'),
    path('api/sync/', views.api_sync_state, name='api_sync'),
    path('api/qrcode/', views.api_qrcode, name='api_qrcode'),
    path('api/advance/', views.api_advance, name='api_advance'),
    path('api/debug/', views.api_debug_state, name='api_debug'),
]
