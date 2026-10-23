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

  Session({required this.role, ApiClient? api}) : api = api ?? ApiClient() {
    realtime = Realtime(() => this.api.wsUri);
    this.api.onUnauthorized = logout;
  }

  String get _tokenKey => 'token_$role';
  bool get loggedIn => user != null;

  /// Restores a saved token and loads the profile. Safe to call once at startup.
  Future<void> init() async {
    startupError = null;
    try {
      config = await api.config();
    } catch (_) {}
    final prefs = await SharedPreferences.getInstance();
    final saved = prefs.getString(_tokenKey);
    if (saved != null && saved.isNotEmpty) {
      api.token = saved;
      try {
        user = await api.me();
        realtime.connect();
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

  Future<void> logout() async {
    realtime.disconnect();
    api.token = null;
    user = null;
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_tokenKey);
    notifyListeners();
  }
}
