import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'api.dart';
import 'models.dart';
import 'realtime.dart';

/// Login state shared by both apps: token persistence, current user, API and WebSocket.
class Session extends ChangeNotifier {
  final String role; // "client" | "driver"
  final ApiClient api;
  late final Realtime realtime;
  User? user;
  AppConfig config = const AppConfig();
  bool ready = false;

  /// Set when a saved session could not be restored because the server is unreachable.
  String? startupError;

  /// Firebase device token (see PushService); sent to the server while logged in.
  String? pushToken;
  bool _loggingOut = false;

  Session({required this.role, ApiClient? api}) : api = api ?? ApiClient() {
    realtime = Realtime(() => this.api.wsUri);
    this.api.onUnauthorized = logout;
  }

  String get _tokenKey => 'token_$role';
  static const _serverKey = 'server_url';

  /// Server address chosen in the app (overrides the build-time default), empty = default.
  Future<String> savedServer() async => (await SharedPreferences.getInstance()).getString(_serverKey) ?? '';

  /// Switches to another backend: forgets the login (tokens are per server) and reloads.
  Future<void> changeServer(String url) async {
    final prefs = await SharedPreferences.getInstance();
    final u = url.trim();
    if (u.isEmpty) {
      await prefs.remove(_serverKey);
    } else {
      await prefs.setString(_serverKey, u);
    }
    realtime.disconnect();
    await prefs.remove(_tokenKey);
    api.token = null;
    user = null;
    api.baseUrl = u.isEmpty ? defaultApiUrl() : u;
    ready = false;
    notifyListeners();
    await init();
  }
  bool get loggedIn => user != null;

  /// Restores a saved token and loads the profile. Safe to call once at startup.
  Future<void> init() async {
    startupError = null;
    final prefs = await SharedPreferences.getInstance();
    final server = prefs.getString(_serverKey) ?? '';
    if (server.isNotEmpty) api.baseUrl = server;
    try {
      config = await api.config();
    } catch (_) {}
    final saved = prefs.getString(_tokenKey);
    if (saved != null && saved.isNotEmpty) {
      api.token = saved;
      try {
        user = await api.me();
        realtime.connect();
        _sendPushToken();
      } on ApiException catch (e) {
        if (e.isUnauthorized || e.status == 403) {
          api.token = null;
          await prefs.remove(_tokenKey);
        } else {
          // offline: keep the token so the user can retry without logging in again
          startupError = e.message;
        }
      }
    }
    ready = true;
    notifyListeners();
  }

  Future<String?> sendOtp(String phone) => api.sendOtp(phone);

  Future<bool> verify(String phone, String code, {String? name}) async {
    final r = await api.verifyOtp(phone, code, role: role, name: name);
    return _loggedIn(r);
  }

  /// Phone + password login (driver app).
  Future<void> loginWithPassword(String phone, String password) async {
    await _loggedIn(await api.passwordLogin(phone, password));
  }

  Future<bool> _loggedIn(({String token, User user, bool isNew}) r) async {
    api.token = r.token;
    user = r.user;
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_tokenKey, r.token);
    realtime.connect();
    _sendPushToken();
    notifyListeners();
    return r.isNew;
  }

  Future<void> refreshUser() async {
    user = await api.me();
    notifyListeners();
  }

  Future<void> updateName(String name) async {
    user = await api.updateProfile(name: name);
    notifyListeners();
  }

  /// Called by PushService on start and whenever Firebase rotates the token.
  void setPushToken(String token) {
    if (token == pushToken) return;
    pushToken = token;
    _sendPushToken();
  }

  void _sendPushToken() {
    final t = pushToken;
    if (t == null || user == null) return;
    api.updateProfile(fcmToken: t).then((_) {}, onError: (_) {});
  }

  Future<void> logout() async {
    if (_loggingOut) return;
    _loggingOut = true;
    // stop notifications for this account on this phone (best effort, token still valid here)
    if (pushToken != null && user != null && api.token != null) {
      try {
        await api.updateProfile(fcmToken: '').timeout(const Duration(seconds: 5));
      } catch (_) {}
    }
    _loggingOut = false;
    realtime.disconnect();
    api.token = null;
    user = null;
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_tokenKey);
    notifyListeners();
  }
}
