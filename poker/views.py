import json
import qrcode
import qrcode.image.svg
from io import BytesIO
from django.shortcuts import render, get_object_or_404
from django.http import JsonResponse, HttpResponse
from django.views.decorators.csrf import csrf_exempt
from django.views.decorators.http import require_http_methods
from .models import Game, Player
from .engine import GameEngine, GameState, generate_table_id

GAME_STATE = GameState()


def index(request):
    table_id = request.GET.get('table_id', generate_table_id())
    return render(request, 'poker/table.html', {
        'table_id': table_id,
    })


def hole_cards(request):
    params = request.GET.get('params', '')
    table_id = request.GET.get('table_id', '')
    parts = params.split('-')
    return render(request, 'poker/hole_cards.html', {
        'params': parts,
        'table_id': table_id,
        'params_raw': params,
    })


@csrf_exempt
@require_http_methods(['POST'])
def api_start_game(request):
    try:
        data = json.loads(request.body)
    except json.JSONDecodeError:
        return JsonResponse({'error': 'Invalid JSON'}, status=400)

    table_id = data.get('table_id', generate_table_id())
    player_names = data.get('players', [])

    engine, created = GAME_STATE.get_or_create(table_id)
    engine.init_game(player_names)

    for p_data in engine.players:
        p_model, _ = Player.objects.update_or_create(
            game=engine.game,
            seat_index=p_data['seat_index'],
            defaults={
                'name': p_data['name'],
                'is_bot': p_data['is_bot'],
                'chips': p_data['chips'],
                'round_bet': 0,
                'total_bet': 0,
                'folded': False,
                'all_in': False,
                'is_dealer': False,
                'is_small_blind': False,
                'is_big_blind': False,
                'stats_data': json.dumps(p_data['stats']),
            },
        )

    engine.start_hand()
    engine.save_state()

    return JsonResponse({
        'ok': True,
        'table_id': table_id,
        'state': engine.to_dict(),
    })


@csrf_exempt
@require_http_methods(['POST'])
def api_act(request):
    try:
        data = json.loads(request.body)
    except json.JSONDecodeError:
        return JsonResponse({'error': 'Invalid JSON'}, status=400)

    table_id = data.get('table_id')
    player_name = data.get('player_name')
    action = data.get('action')
    amount = data.get('amount', 0)

    if not all([table_id, player_name, action]):
        return JsonResponse({'error': 'Missing fields'}, status=400)

    engine, _ = GAME_STATE.get_or_create(table_id)
    engine.load_from_model(engine.game)

    success = engine.human_action(player_name, action, amount)
    if not success:
        return JsonResponse({'error': 'Invalid action'}, status=400)

    return JsonResponse({
        'ok': True,
        'state': engine.to_dict(),
    })


@csrf_exempt
@require_http_methods(['POST'])
def api_advance(request):
    try:
        data = json.loads(request.body)
    except json.JSONDecodeError:
        return JsonResponse({'error': 'Invalid JSON'}, status=400)

    table_id = data.get('table_id')
    if not table_id:
        return JsonResponse({'error': 'Missing table_id'}, status=400)

    engine, _ = GAME_STATE.get_or_create(table_id)
    engine.load_from_model(engine.game)

    engine.advance_one_step()

    return JsonResponse({
        'ok': True,
        'state': engine.to_dict(),
        'version': engine.game.version if engine.game else 0,
    })


@require_http_methods(['GET'])
def api_state(request):
    table_id = request.GET.get('table_id')
    since_version = request.GET.get('since_version', '0')

    if not table_id:
        return JsonResponse({'error': 'Missing table_id'}, status=400)

    try:
        game_model = Game.objects.get(table_id=table_id)
    except Game.DoesNotExist:
        return JsonResponse({'error': 'Not found'}, status=404)

    since_version = int(since_version) if since_version.isdigit() else 0

    if game_model.version <= since_version:
        return HttpResponse(status=204)

    engine, _ = GAME_STATE.get_or_create(table_id)
    engine.load_from_model(game_model)

    return JsonResponse({
        'version': game_model.version,
        'state': engine.to_dict(),
        'notifications': engine.notifications[-8:],
    })


@csrf_exempt
@require_http_methods(['POST'])
def api_sync_state(request):
    try:
        data = json.loads(request.body)
    except json.JSONDecodeError:
        return JsonResponse({'error': 'Invalid JSON'}, status=400)

    table_id = data.get('table_id', 'default')
    state_data = data.get('state')
    notifications = data.get('notifications', [])

    if not state_data:
        return JsonResponse({'error': 'Missing state'}, status=400)

    game_model, _ = Game.objects.get_or_create(
        table_id=table_id,
        defaults={'small_blind': 10, 'big_blind': 20, 'last_raise': 20},
    )

    import json as j
    if 'community_cards' in state_data:
        game_model.community_cards = j.dumps(state_data['community_cards'])
    if 'pot' in state_data:
        game_model.pot = state_data['pot']
    if 'current_bet' in state_data:
        game_model.current_bet = state_data['current_bet']
    if 'phase' in state_data:
        game_model.phase = state_data['phase']
    game_model.version += 1
    if notifications:
        game_model.notifications = j.dumps(notifications)
    game_model.save()

    engine = GAME_STATE.engines.get(table_id)
    if engine:
        engine.load_from_model(game_model)

    return JsonResponse({
        'ok': True,
        'state': engine.to_dict(),
    })


@require_http_methods(['GET'])
def api_qrcode(request):
    text = request.GET.get('text', '')
    if not text:
        return JsonResponse({'error': 'Missing text'}, status=400)

    factory = qrcode.image.svg.SvgImage
    img = qrcode.make(text, image_factory=factory)
    buf = BytesIO()
    img.save(buf)
    return HttpResponse(buf.getvalue(), content_type='image/svg+xml')


def api_debug_state(request):
    engine = GAME_STATE.engines.get(request.GET.get('table_id', ''))
    if not engine:
        return JsonResponse({'error': 'No engine'}, status=404)
    return JsonResponse(engine.to_dict())
