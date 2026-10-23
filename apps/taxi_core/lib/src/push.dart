import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';

import 'session.dart';

/// Firebase settings from the build (`--dart-define=FIREBASE_API_KEY=...` etc.).
/// Without them push is simply off and the app relies on the WebSocket alone.
class PushConfig {
  static const apiKey = String.fromEnvironment('FIREBASE_API_KEY');
  static const appId = String.fromEnvironment('FIREBASE_APP_ID');
  static const senderId = String.fromEnvironment('FIREBASE_SENDER_ID');
  static const projectId = String.fromEnvironment('FIREBASE_PROJECT_ID');
  static const iosBundleId = String.fromEnvironment('FIREBASE_IOS_BUNDLE_ID');

  /// Web push needs a service worker and VAPID key; the web apps stay on WebSocket only.
  static bool get enabled => !kIsWeb && apiKey.isNotEmpty && appId.isNotEmpty && senderId.isNotEmpty && projectId.isNotEmpty;

  static FirebaseOptions get options => FirebaseOptions(
        apiKey: apiKey,
        appId: appId,
        messagingSenderId: senderId,
        projectId: projectId,
        iosBundleId: iosBundleId.isEmpty ? null : iosBundleId,
      );
}

/// Registers the device for Firebase Cloud Messaging and hands the token to [Session],
/// which stores it on the server for the logged-in account.
class PushService {
  static bool _started = false;

  static Future<void> start(Session session) async {
    if (_started || !PushConfig.enabled) return;
    _started = true;
    try {
      await Firebase.initializeApp(options: PushConfig.options);
      final m = FirebaseMessaging.instance;
      // Android 13+ / iOS ask the user; older Android grants it at install
      await m.requestPermission();
      await m.setForegroundNotificationPresentationOptions(alert: false, badge: true, sound: false);
      // Registration tokens are what the backend's FCM HTTP v1 "message.token" expects.
      // The newer register()/onRegistered flow yields installation IDs instead; keep tokens
      // until the backend sends to installation IDs.
      // ignore: deprecated_member_use
      final token = await m.getToken();
      if (token != null) session.setPushToken(token);
      // ignore: deprecated_member_use
      m.onTokenRefresh.listen(session.setPushToken);
    } catch (e) {
      debugPrint('push disabled: $e');
    }
  }
}
