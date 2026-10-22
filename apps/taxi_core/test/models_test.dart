import 'package:flutter_test/flutter_test.dart';
import 'package:taxi_core/taxi_core.dart';

void main() {
  group('Order.fromJson', () {
    // Shape produced by the Go backend (handlers.loadOrder with relations).
    final json = <String, dynamic>{
      'id': 12,
      'client_id': 3,
      'client': {'id': 3, 'phone': '+99365000001', 'name': 'Aman', 'role': 'client', 'rating': 4.8, 'rating_count': 5},
      'driver_id': 7,
      'driver': {
        'id': 7,
        'user_id': 4,
        'user': {'id': 4, 'phone': '+99362000001', 'name': 'Merdan', 'role': 'driver', 'rating': 5},
        'status': 'approved',
        'is_online': true,
        'lat': 37.95,
        'lng': 58.33,
        'car_brand': 'Toyota',
        'car_model': 'Camry',
        'car_color': 'Ak',
        'car_plate': 'AG 1234',
        'tariff_codes': 'economy,comfort',
        'current_order_id': 12,
      },
      'tariff_id': 1,
      'tariff': {'id': 1, 'code': 'economy', 'name': 'Ekonom', 'free_wait_min': 3},
      'status': 'accepted',
      'order_type': 'ride',
      'pickup_lat': 37.96,
      'pickup_lng': 58.326,
      'pickup_address': 'Bitarap Türkmenistan şaýoly',
      'dropoff_lat': 37.93,
      'dropoff_lng': 58.39,
      'dropoff_address': 'Aeroport',
      'distance_km': 7.4,
      'estimated_price': 21.5,
      'discount': 2,
      'final_price': 19.5,
      'payment_method': 'cash',
      'created_at': '2026-10-22T10:00:00+05:00',
      'accepted_at': '2026-10-22T10:01:00+05:00',
      'arrived_at': null,
      'completed_at': '0001-01-01T00:00:00Z',
    };

    test('parses relations and nested driver user', () {
      final o = Order.fromJson(json);
      expect(o.id, 12);
      expect(o.client!.name, 'Aman');
      expect(o.driver!.user!.name, 'Merdan');
      expect(o.driver!.carTitle, 'Ak Toyota Camry');
      expect(o.driver!.tariffList, ['economy', 'comfort']);
      expect(o.tariff!.freeWaitMin, 3);
      expect(o.isActive, isTrue);
      expect(o.price, 19.5);
    });

    test('treats null and Go zero time as unset', () {
      final o = Order.fromJson(json);
      expect(o.acceptedAt, isNotNull);
      expect(o.arrivedAt, isNull);
      expect(o.completedAt, isNull);
    });

    test('price falls back to estimate minus discount', () {
      final o = Order.fromJson({...json, 'final_price': 0});
      expect(o.price, 19.5);
    });

    test('survives missing fields', () {
      final o = Order.fromJson({'id': 1});
      expect(o.status, '');
      expect(o.paymentMethod, 'cash');
      expect(o.driver, isNull);
    });
  });

  test('Estimate parses options and polyline', () {
    final e = Estimate.fromJson({
      'distance_km': 5.5,
      'duration_min': 11,
      'polyline': [
        {'lat': 37.9, 'lng': 58.3},
        {'lat': 37.91, 'lng': 58.31},
      ],
      'options': [
        {
          'tariff': {'id': 1, 'code': 'economy', 'name': 'Ekonom'},
          'price': 14.5,
        },
      ],
      'drivers_nearby': 2,
      'pickup_eta_min': 4,
      'currency': 'TMT',
    });
    expect(e.polyline, hasLength(2));
    expect(e.options.single.price, 14.5);
    expect(e.options.single.tariff.code, 'economy');
  });

  test('PromoResult.discountFor mirrors the backend formula', () {
    const p = PromoResult('SALAM10', 0, percent: 10);
    expect(p.discountFor(23), 2.5); // 2.3 rounded to 0.5
    const f = PromoResult('FIX', 0, fixed: 50);
    expect(f.discountFor(20), 20); // never more than the price
  });

  group('phone', () {
    test('normalizes like the backend', () {
      expect(normalizePhone('65 12-34-56'), '+99365123456');
      expect(normalizePhone('865123456'), '+99365123456');
      expect(normalizePhone('+993 65 123456'), '+99365123456');
      expect(validPhone(normalizePhone('12345')), isFalse);
    });

    test('pretty prints', () {
      expect(prettyPhone('+99365123456'), '+993 65 12-34-56');
    });
  });

  test('money and km formatting', () {
    expect(money(12), '12 TMT');
    expect(money(12.5), '12.5 TMT');
    expect(km(0.35), '350 m');
    expect(km(7.42), '7.4 km');
  });

  test('ApiClient builds the websocket url from the base url', () {
    final api = ApiClient(baseUrl: 'https://api.taxi.tm/', token: 'abc');
    expect(api.wsUri.toString(), 'wss://api.taxi.tm/ws?token=abc');
    final local = ApiClient(baseUrl: 'http://10.0.2.2:8080', token: 't');
    expect(local.wsUri.toString(), 'ws://10.0.2.2:8080/ws?token=t');
  });
}
