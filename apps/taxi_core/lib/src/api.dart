import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import 'models.dart';

/// Base URL of the backend, e.g. `http://192.168.1.10:8080`.
/// Set at build time: `flutter run --dart-define=API_URL=https://api.example.tm`.
/// The Android emulator reaches the host machine at 10.0.2.2.
const String kApiUrl = String.fromEnvironment('API_URL', defaultValue: 'http://10.0.2.2:8080');

class ApiException implements Exception {
  final int status;
  final String message;

  /// Raw `data` of the error envelope (e.g. the active order on HTTP 409).
  final dynamic data;

  const ApiException(this.status, this.message, [this.data]);

  bool get isUnauthorized => status == 401;
  bool get isNetwork => status == 0;

  @override
  String toString() => message;
}

/// Thin JSON client for the `/api/v1` envelope `{success, data, error}`.
class ApiClient {
  final String baseUrl;
  final http.Client _http;
  String? token;

  /// Called when the server rejects the token so the app can return to login.
  void Function()? onUnauthorized;

  ApiClient({String? baseUrl, http.Client? client, this.token})
      : baseUrl = (baseUrl ?? kApiUrl).replaceAll(RegExp(r'/+$'), ''),
        _http = client ?? http.Client();

  Uri _uri(String path, [Map<String, dynamic>? query]) {
    final q = query?.map((k, v) => MapEntry(k, '$v'));
    return Uri.parse('$baseUrl/api/v1$path').replace(queryParameters: q == null || q.isEmpty ? null : q);
  }

  /// WebSocket endpoint for the current token.
  Uri get wsUri {
    final u = Uri.parse(baseUrl);
    return u.replace(scheme: u.scheme == 'https' ? 'wss' : 'ws', path: '/ws', queryParameters: {'token': token ?? ''});
  }

  Future<dynamic> _send(String method, String path, {Object? body, Map<String, dynamic>? query}) async {
    final req = http.Request(method, _uri(path, query));
    req.headers['Accept'] = 'application/json';
    if (token != null && token!.isNotEmpty) req.headers['Authorization'] = 'Bearer $token';
    if (body != null) {
      req.headers['Content-Type'] = 'application/json';
      req.body = jsonEncode(body);
    }
    http.Response res;
    try {
      res = await http.Response.fromStream(await _http.send(req).timeout(const Duration(seconds: 20)));
    } on TimeoutException {
      throw const ApiException(0, 'Serwer jogap bermedi. Internedi barlaň.');
    } catch (_) {
      throw const ApiException(0, 'Serwere birikip bolmady. Internedi barlaň.');
    }
    Map<String, dynamic> j;
    try {
      j = (jsonDecode(utf8.decode(res.bodyBytes)) as Map).cast<String, dynamic>();
    } catch (_) {
      throw ApiException(res.statusCode, 'Serwer ýalňyşlygy (${res.statusCode})');
    }
    if (j['success'] != true) {
      if (res.statusCode == 401 && token != null) onUnauthorized?.call();
      throw ApiException(res.statusCode, (j['error'] ?? 'Ýalňyşlyk').toString(), j['data']);
    }
    return j['data'];
  }

  Future<dynamic> get(String path, [Map<String, dynamic>? query]) => _send('GET', path, query: query);
  Future<dynamic> post(String path, [Object? body]) => _send('POST', path, body: body ?? const {});
  Future<dynamic> put(String path, [Object? body]) => _send('PUT', path, body: body ?? const {});
  Future<dynamic> delete(String path) => _send('DELETE', path);

  static Map<String, dynamic> _map(dynamic v) => (v as Map).cast<String, dynamic>();
  static List<T> _list<T>(dynamic v, T Function(Map<String, dynamic>) f) =>
      v is List ? v.whereType<Map>().map((e) => f(e.cast<String, dynamic>())).toList() : <T>[];

  // ---------- public ----------
  Future<AppConfig> config() async => AppConfig.fromJson(_map(await get('/config')));
  Future<List<Tariff>> tariffs() async => _list(await get('/tariffs'), Tariff.fromJson);
  Future<Place?> reverse(double lat, double lng) async {
    final d = await get('/geo/reverse', {'lat': lat, 'lng': lng});
    return d is Map ? Place.fromJson(_map(d)) : null;
  }

  Future<List<Place>> search(String q) async => _list(await get('/geo/search', {'q': q}), Place.fromJson);
  Future<List<NearbyCar>> nearbyDrivers(double lat, double lng) async =>
      _list(await get('/drivers/nearby', {'lat': lat, 'lng': lng}), NearbyCar.fromJson);

  // ---------- auth ----------
  /// Returns the dev code when the server runs in development mode (for testing).
  Future<String?> sendOtp(String phone) async {
    final d = _map(await post('/auth/send-otp', {'phone': phone}));
    return d['dev_code']?.toString();
  }

  /// Logs in; [role] is "client" or "driver" depending on the app.
  Future<({String token, User user, bool isNew})> verifyOtp(String phone, String code, {required String role, String? name}) async {
    final d = _map(await post('/auth/verify-otp', {
      'phone': phone,
      'code': code,
      'role': role,
      if (name != null && name.isNotEmpty) 'name': name,
      'lang': 'tk',
    }));
    return (token: d['token'].toString(), user: User.fromJson(_map(d['user'])), isNew: d['is_new'] == true);
  }

  Future<User> me() async => User.fromJson(_map(await get('/auth/me')));
  Future<User> updateProfile({String? name, String? lang, String? fcmToken}) async => User.fromJson(_map(await put('/auth/profile', {
        'name': ?name,
        'lang': ?lang,
        'fcm_token': ?fcmToken,
      })));

  // ---------- client ----------
  Future<Estimate> estimate(Place pickup, Place dropoff) async =>
      Estimate.fromJson(_map(await post('/orders/estimate', {'pickup': pickup.toPointJson(), 'dropoff': dropoff.toPointJson()})));

  Future<Order> createOrder({
    required int tariffId,
    required Place pickup,
    required Place dropoff,
    String paymentMethod = 'cash',
    String? promoCode,
    String? comment,
    String? recipientPhone,
  }) async =>
      Order.fromJson(_map(await post('/orders/', {
        'tariff_id': tariffId,
        'pickup': pickup.toPointJson(),
        'dropoff': dropoff.toPointJson(),
        'payment_method': paymentMethod,
        if (promoCode != null && promoCode.isNotEmpty) 'promo_code': promoCode,
        if (comment != null && comment.isNotEmpty) 'comment': comment,
        if (recipientPhone != null && recipientPhone.isNotEmpty) 'recipient_phone': recipientPhone,
      })));

  Future<Order?> activeOrder() async {
    final d = await get('/orders/active');
    return d is Map ? Order.fromJson(_map(d)) : null;
  }

  Future<List<Order>> myOrders({int page = 1}) async => _list(await get('/orders/', {'page': page, 'limit': 30}), Order.fromJson);
  Future<Order> order(int id) async => Order.fromJson(_map(await get('/orders/$id')));
  Future<void> cancelOrder(int id, [String reason = '']) => post('/orders/$id/cancel', {'reason': reason});
  Future<void> rateDriver(int id, int rating, [String comment = '']) => post('/orders/$id/rate', {'rating': rating, 'comment': comment});

  Future<List<SavedAddress>> addresses() async => _list(await get('/addresses'), SavedAddress.fromJson);
  Future<SavedAddress> addAddress(String title, Place p) async =>
      SavedAddress.fromJson(_map(await post('/addresses', {'title': title, 'address': p.title, 'lat': p.lat, 'lng': p.lng})));
  Future<void> deleteAddress(int id) => delete('/addresses/$id');
  Future<PromoResult> checkPromo(String code, double price) async =>
      PromoResult.fromJson(_map(await post('/promo/check', {'code': code, 'price': price})));

  Future<SupportTicket> createTicket(String message, {int? orderId}) async =>
      SupportTicket.fromJson(_map(await post('/support', {'message': message, 'order_id': ?orderId})));
  Future<List<SupportTicket>> myTickets() async => _list(await get('/support'), SupportTicket.fromJson);

  // ---------- driver ----------
  Future<DriverProfile> driverRegister({
    required String name,
    required String carBrand,
    required String carModel,
    required String carColor,
    required String carPlate,
    required int carYear,
    required String licenseNumber,
    required List<String> tariffCodes,
  }) async =>
      DriverProfile.fromJson(_map(await post('/driver/register', {
        'name': name,
        'car_brand': carBrand,
        'car_model': carModel,
        'car_color': carColor,
        'car_plate': carPlate,
        'car_year': carYear,
        'license_number': licenseNumber,
        'tariff_codes': tariffCodes.join(','),
      })));

  /// Returns null when the driver has not registered a car yet (HTTP 404).
  Future<({DriverProfile driver, Order? activeOrder})?> driverMe() async {
    try {
      final d = _map(await get('/driver/me'));
      final ao = d['active_order'];
      return (driver: DriverProfile.fromJson(_map(d['driver'])), activeOrder: ao is Map ? Order.fromJson(_map(ao)) : null);
    } on ApiException catch (e) {
      if (e.status == 404) return null;
      rethrow;
    }
  }

  Future<bool> setOnline(bool online, {double? lat, double? lng}) async {
    final d = _map(await post('/driver/online', {'online': online, 'lat': ?lat, 'lng': ?lng}));
    return d['online'] == true;
  }

  Future<void> sendLocation(double lat, double lng, double heading) =>
      post('/driver/location', {'lat': lat, 'lng': lng, 'heading': heading});
  Future<List<OrderOffer>> availableOrders() async => _list(await get('/driver/orders/available'), OrderOffer.fromJson);
  Future<Order> acceptOrder(int id) async => Order.fromJson(_map(await post('/driver/orders/$id/accept')));
  Future<void> declineOrder(int id) => post('/driver/orders/$id/decline');
  Future<Order> arrived(int id) async => Order.fromJson(_map(await post('/driver/orders/$id/arrived')));
  Future<Order> startRide(int id) async => Order.fromJson(_map(await post('/driver/orders/$id/start')));
  Future<Order> completeRide(int id) async => Order.fromJson(_map(await post('/driver/orders/$id/complete')));
  Future<void> driverCancel(int id, [String reason = '']) => post('/driver/orders/$id/cancel', {'reason': reason});
  Future<void> rateClient(int id, int rating) => post('/driver/orders/$id/rate', {'rating': rating});
  Future<List<Order>> driverOrders({int page = 1}) async =>
      _list(await get('/driver/orders', {'page': page, 'limit': 30}), Order.fromJson);
  Future<Earnings> earnings() async => Earnings.fromJson(_map(await get('/driver/earnings')));
}
