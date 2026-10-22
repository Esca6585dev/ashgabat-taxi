import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:taxi_core/taxi_core.dart';

/// Something the UI should tell the user once (ride finished, driver cancelled...).
class RideNotice {
  final Order order;
  final String kind; // completed | cancelled | requeued
  final String reason;
  const RideNotice(this.order, this.kind, [this.reason = '']);
}

/// Tracks the client's active order from WebSocket events with a polling fallback.
class RideController extends ChangeNotifier {
  final Session session;
  Order? order;
  LatLngPoint? driverPos;
  double driverHeading = 0;
  final _notices = StreamController<RideNotice>.broadcast();
  StreamSubscription? _evSub;
  StreamSubscription? _connSub;
  Timer? _poll;
  bool _disposed = false;

  RideController(this.session) {
    _evSub = session.realtime.events.listen(_onEvent);
    // events may have been missed while offline -> resync on every reconnect
    _connSub = session.realtime.connection.listen((up) {
      if (up) refresh();
    });
    _poll = Timer.periodic(const Duration(seconds: 15), (_) {
      if (order != null) refresh();
    });
    refresh();
  }

  Stream<RideNotice> get notices => _notices.stream;
  bool get hasOrder => order != null;

  void _set(Order? o) {
    if (_disposed) return;
    final prevDriver = order?.driverId;
    order = o;
    if (o == null || o.driverId == null) {
      driverPos = null;
    } else if (o.driverId != prevDriver || driverPos == null) {
      final d = o.driver;
      if (d != null && (d.lat != 0 || d.lng != 0)) {
        driverPos = LatLngPoint(d.lat, d.lng);
        driverHeading = d.heading;
      }
    }
    notifyListeners();
  }

  void _finish(Order o, String kind, [String reason = '']) {
    _set(null);
    if (!_notices.isClosed) _notices.add(RideNotice(o, kind, reason));
  }

  /// Called right after the order was created.
  void started(Order o) => _set(o);

  Future<void> refresh() async {
    try {
      final o = await session.api.activeOrder();
      if (o != null) {
        _set(o);
      } else if (order != null) {
        // the order ended while we were not listening -> fetch its final state
        final last = await session.api.order(order!.id);
        _finish(last, last.status == OrderStatus.completed ? 'completed' : 'cancelled', last.cancelReason);
      }
    } catch (_) {}
  }

  void _onEvent(WsEvent ev) {
    switch (ev.type) {
      case 'order.accepted':
      case 'order.status':
      case 'order.completed':
      case 'order.cancelled':
        final o = Order.fromJson(ev.map);
        // ignore events of other orders, and late events of an order we already closed
        if (order == null ? !o.isActive : o.id != order!.id) return;
        if (o.status == OrderStatus.completed) {
          _finish(o, 'completed');
        } else if (o.status == OrderStatus.cancelled) {
          _finish(o, 'cancelled', o.cancelReason);
        } else {
          _set(o);
        }
      case 'order.requeued':
        final m = ev.map['order'];
        if (m is Map) {
          final o = Order.fromJson(m.cast<String, dynamic>());
          _set(o);
          if (!_notices.isClosed) _notices.add(RideNotice(o, 'requeued', '${ev.map['reason'] ?? ''}'));
        }
      case 'driver.location':
        final m = ev.map;
        if (order != null && m['order_id'] == order!.id) {
          driverPos = LatLngPoint((m['lat'] as num).toDouble(), (m['lng'] as num).toDouble());
          driverHeading = (m['heading'] as num?)?.toDouble() ?? 0;
          notifyListeners();
        }
      case 'account.blocked':
        session.logout();
    }
  }

  Future<void> cancel(String reason) async {
    final o = order;
    if (o == null) return;
    await session.api.cancelOrder(o.id, reason);
    _set(null); // our own cancellation needs no notice
  }

  @override
  void dispose() {
    _disposed = true;
    _evSub?.cancel();
    _connSub?.cancel();
    _poll?.cancel();
    _notices.close();
    super.dispose();
  }
}
