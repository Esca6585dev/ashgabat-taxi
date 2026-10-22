import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

/// A server push `{"type": "...", "data": ..., "at": unix}`.
///
/// Types sent by the backend: connected, order.new, order.taken, order.accepted,
/// order.status, order.completed, order.cancelled, order.requeued,
/// driver.location, driver.status, balance.changed, ticket.reply, account.blocked, pong.
class WsEvent {
  final String type;
  final dynamic data;
  const WsEvent(this.type, this.data);

  Map<String, dynamic> get map => data is Map ? (data as Map).cast<String, dynamic>() : const {};
}

/// Keeps one WebSocket open to `/ws?token=` and reconnects with back-off.
class Realtime {
  final Uri Function() _uri;
  final _events = StreamController<WsEvent>.broadcast();
  final _connected = StreamController<bool>.broadcast();
  WebSocketChannel? _ch;
  StreamSubscription? _sub;
  Timer? _ping;
  Timer? _retry;
  int _attempt = 0;
  bool _wanted = false;
  bool isConnected = false;

  Realtime(this._uri);

  Stream<WsEvent> get events => _events.stream;
  Stream<bool> get connection => _connected.stream;

  void connect() {
    _wanted = true;
    if (_ch != null) return;
    _open();
  }

  Future<void> _open() async {
    _retry?.cancel();
    final ch = WebSocketChannel.connect(_uri());
    _ch = ch;
    try {
      await ch.ready;
    } catch (_) {
      _closed(ch);
      return;
    }
    if (!_wanted || _ch != ch) {
      ch.sink.close();
      return;
    }
    _attempt = 0;
    _setConnected(true);
    _sub = ch.stream.listen(
      (msg) {
        try {
          final j = (jsonDecode(msg as String) as Map).cast<String, dynamic>();
          _events.add(WsEvent(j['type']?.toString() ?? '', j['data']));
        } catch (_) {}
      },
      onDone: () => _closed(ch),
      onError: (_) => _closed(ch),
      cancelOnError: true,
    );
    _ping?.cancel();
    _ping = Timer.periodic(const Duration(seconds: 25), (_) => send({'type': 'ping'}));
  }

  void _closed(WebSocketChannel ch) {
    if (_ch != ch) return;
    _ch = null;
    _sub?.cancel();
    _ping?.cancel();
    _setConnected(false);
    if (!_wanted) return;
    final delay = Duration(seconds: [1, 2, 4, 8, 15][_attempt.clamp(0, 4)]);
    _attempt++;
    _retry = Timer(delay, _open);
  }

  void _setConnected(bool v) {
    isConnected = v;
    if (!_connected.isClosed) _connected.add(v);
  }

  /// Sends a JSON message; silently dropped while disconnected.
  bool send(Map<String, dynamic> msg) {
    final ch = _ch;
    if (ch == null || !isConnected) return false;
    ch.sink.add(jsonEncode(msg));
    return true;
  }

  bool sendLocation(double lat, double lng, double heading) =>
      send({'type': 'location', 'lat': lat, 'lng': lng, 'heading': heading});

  void disconnect() {
    _wanted = false;
    _retry?.cancel();
    _ping?.cancel();
    _sub?.cancel();
    _ch?.sink.close();
    _ch = null;
    _setConnected(false);
  }

  void dispose() {
    disconnect();
    _events.close();
    _connected.close();
  }
}
