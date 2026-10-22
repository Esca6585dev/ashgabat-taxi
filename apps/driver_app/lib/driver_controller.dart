import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:geolocator/geolocator.dart';
import 'package:taxi_core/taxi_core.dart';

/// Something the UI should tell the driver once.
class DriverNotice {
  final String kind; // cancelled | taken | status
  final String text;
  final Order? order;
  const DriverNotice(this.kind, this.text, [this.order]);
}

/// Driver state: profile, online switch, GPS tracking, incoming offers and the current order.
class DriverController extends ChangeNotifier {
  final Session session;
  DriverProfile? driver;
  bool loaded = false;
  String? loadError;
  Order? order;
  final offers = <OrderOffer>[];
  LatLngPoint? pos;
  double heading = 0;
  bool online = false;
  bool switching = false;

  /// Seconds an offer stays on screen before it is auto-declined.
  static const offerSeconds = 30;

  final _notices = StreamController<DriverNotice>.broadcast();
  final _skipped = <int>{}; // declined or expired order ids
  StreamSubscription? _ev;
  StreamSubscription? _conn;
  StreamSubscription<Position>? _gps;
  Timer? _heartbeat;
  Timer? _poll;
  Timer? _expiry;
  bool _disposed = false;

  DriverController(this.session) {
    _ev = session.realtime.events.listen(_onEvent);
    _conn = session.realtime.connection.listen((up) {
      if (up && loaded) refresh();
    });
    _expiry = Timer.periodic(const Duration(seconds: 1), (_) => _expireOffers());
  }

  ApiClient get api => session.api;
  Stream<DriverNotice> get notices => _notices.stream;
  bool get approved => driver?.status == DriverStatus.approved;
  bool get busy => order != null;

  void _notify() {
    if (!_disposed) notifyListeners();
  }

  void _say(String kind, String text, [Order? o]) {
    if (!_notices.isClosed) _notices.add(DriverNotice(kind, text, o));
  }

  /// Loads /driver/me. `driver == null` afterwards means "not registered yet".
  Future<void> refresh() async {
    try {
      final r = await api.driverMe();
      loadError = null;
      driver = r?.driver;
      order = r?.activeOrder;
      online = driver?.isOnline == true;
      if (online || order != null) {
        _startTracking();
      } else {
        _stopTracking();
      }
      if (order != null) offers.clear();
    } on ApiException catch (e) {
      loadError = e.message;
    }
    loaded = true;
    _notify();
  }

  void registered(DriverProfile d) {
    driver = d;
    _notify();
  }

  // ---------- online / GPS ----------
  Future<void> setOnline(bool v) async {
    if (switching) return;
    switching = true;
    _notify();
    try {
      LatLngPoint? p;
      if (v) {
        final problem = await LocationService.ensurePermission();
        if (problem != null) throw ApiException(0, problem);
        p = await LocationService.currentPoint();
        if (p == null) throw const ApiException(0, 'GPS ýerleşişi alynmady. Daşarda synanyşyň.');
        pos = p;
      }
      online = await api.setOnline(v, lat: p?.lat, lng: p?.lng);
      if (online) {
        _startTracking();
        _pollOffers();
      } else {
        offers.clear();
        if (order == null) _stopTracking();
      }
    } finally {
      switching = false;
      _notify();
    }
  }

  void _startTracking() {
    _gps ??= LocationService.stream(distanceFilter: 15).listen((p) {
      pos = LatLngPoint(p.latitude, p.longitude);
      if (p.heading >= 0) heading = p.heading;
      _sendPos();
      _notify();
    }, onError: (_) {});
    // the server marks a silent driver offline, so re-send the position even when standing still
    _heartbeat ??= Timer.periodic(const Duration(seconds: 30), (_) => _sendPos());
    _poll ??= Timer.periodic(const Duration(seconds: 12), (_) => _pollOffers());
    if (pos == null) {
      LocationService.currentPoint().then((p) {
        if (p == null) return;
        pos = p;
        _sendPos();
        _notify();
      });
    }
  }

  void _stopTracking() {
    _gps?.cancel();
    _gps = null;
    _heartbeat?.cancel();
    _heartbeat = null;
    _poll?.cancel();
    _poll = null;
  }

  void _sendPos() {
    final p = pos;
    if (p == null || (!online && order == null)) return;
    if (!session.realtime.sendLocation(p.lat, p.lng, heading)) {
      api.sendLocation(p.lat, p.lng, heading).catchError((_) {});
    }
  }

  // ---------- offers ----------
  /// Fallback for missed WebSocket pushes: lists searching orders nearby.
  Future<void> _pollOffers() async {
    if (!online || order != null) return;
    try {
      final list = await api.availableOrders();
      var changed = false;
      for (final o in list) {
        if (_skipped.contains(o.order.id) || offers.any((x) => x.order.id == o.order.id)) continue;
        offers.add(o);
        changed = true;
      }
      // drop offers the server no longer lists (taken / cancelled)
      final ids = list.map((e) => e.order.id).toSet();
      final before = offers.length;
      offers.removeWhere((x) => !ids.contains(x.order.id));
      if (changed || before != offers.length) _notify();
    } catch (_) {}
  }

  void _expireOffers() {
    if (offers.isEmpty) return;
    final now = DateTime.now();
    final old = offers.where((o) => now.difference(o.receivedAt).inSeconds >= offerSeconds).toList();
    for (final o in old) {
      decline(o);
    }
    _notify(); // countdown redraw
  }

  Future<void> accept(OrderOffer o) async {
    try {
      order = await api.acceptOrder(o.order.id);
      offers.clear();
      _startTracking();
    } on ApiException {
      offers.removeWhere((x) => x.order.id == o.order.id);
      _skipped.add(o.order.id);
      rethrow;
    } finally {
      _notify();
    }
  }

  void decline(OrderOffer o) {
    _skipped.add(o.order.id);
    offers.removeWhere((x) => x.order.id == o.order.id);
    api.declineOrder(o.order.id).catchError((_) {});
    _notify();
  }

  // ---------- ride steps ----------
  Future<void> arrived() async {
    order = await api.arrived(order!.id);
    _notify();
  }

  Future<void> start() async {
    order = await api.startRide(order!.id);
    _notify();
  }

  /// Completes the ride and returns the final order (with fare and commission).
  Future<Order> complete() async {
    final o = await api.completeRide(order!.id);
    order = null;
    if (!online) _stopTracking();
    _notify();
    refreshProfile();
    return o;
  }

  Future<void> cancel(String reason) async {
    final id = order!.id;
    await api.driverCancel(id, reason);
    _skipped.add(id);
    order = null;
    if (!online) _stopTracking();
    _notify();
  }

  Future<void> refreshProfile() async {
    try {
      final r = await api.driverMe();
      if (r != null) driver = r.driver;
      _notify();
    } catch (_) {}
  }

  // ---------- realtime ----------
  void _onEvent(WsEvent ev) {
    final m = ev.map;
    switch (ev.type) {
      case 'order.new':
        if (!online || order != null) return;
        final o = OrderOffer.fromJson(m);
        if (_skipped.contains(o.order.id) || offers.any((x) => x.order.id == o.order.id)) return;
        offers.add(o);
        _say('new', 'Täze sargyt!', o.order);
        _notify();
      case 'order.taken':
        final id = m['order_id'];
        offers.removeWhere((x) => x.order.id == id);
        _notify();
      case 'order.cancelled':
        final o = Order.fromJson(m);
        offers.removeWhere((x) => x.order.id == o.id);
        if (order?.id == o.id) {
          order = null;
          if (!online) _stopTracking();
          _say('cancelled', o.cancelledBy == 'admin' ? 'Sargydy operator ýatyrdy' : 'Müşderi sargydy ýatyrdy', o);
        }
        _notify();
      case 'order.accepted':
      case 'order.status':
        final o = Order.fromJson(m);
        // admin may assign an order directly to this driver
        if (o.driverId != null && o.driverId == driver?.id && o.isActive) {
          final fresh = order == null;
          order = o;
          offers.clear();
          _startTracking();
          if (fresh) _say('assigned', 'Size sargyt bellendi', o);
          _notify();
        }
      case 'driver.status':
        driver = DriverProfile.fromJson(m);
        if (!approved) {
          online = false;
          offers.clear();
          if (order == null) _stopTracking();
        }
        _say('status', 'Profiliňiziň ýagdaýy üýtgedi');
        _notify();
      case 'balance.changed':
        refreshProfile();
      case 'account.blocked':
        session.logout();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    _stopTracking();
    _ev?.cancel();
    _conn?.cancel();
    _expiry?.cancel();
    _notices.close();
    super.dispose();
  }
}
