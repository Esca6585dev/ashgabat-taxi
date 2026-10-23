import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:geolocator/geolocator.dart';

import 'models.dart';

/// Wrapper over geolocator that never throws: returns null when the
/// permission is denied or GPS is off, so the UI can fall back to the city centre.
class LocationService {
  /// Asks for permission if needed. Returns a human readable problem or null when OK.
  static Future<String?> ensurePermission() async {
    try {
      if (!await Geolocator.isLocationServiceEnabled()) return 'GPS öçük. Sazlamalarda ýerleşişi açyň.';
      var p = await Geolocator.checkPermission();
      if (p == LocationPermission.denied) p = await Geolocator.requestPermission();
      if (p == LocationPermission.denied) return 'Ýerleşiş rugsady berilmedi.';
      if (p == LocationPermission.deniedForever) return 'Ýerleşiş rugsady öçürilen. Programmanyň sazlamalarynda açyň.';
      return null;
    } catch (_) {
      return 'Ýerleşişi kesgitläp bolmady.';
    }
  }

  static Future<Position?> current() async {
    if (await ensurePermission() != null) return null;
    try {
      return await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(accuracy: LocationAccuracy.high, timeLimit: Duration(seconds: 15)),
      );
    } catch (_) {
      try {
        return await Geolocator.getLastKnownPosition();
      } catch (_) {
        return null;
      }
    }
  }

  static Future<LatLngPoint?> currentPoint() async {
    final p = await current();
    return p == null ? null : LatLngPoint(p.latitude, p.longitude);
  }

  /// Position updates every [distanceFilter] metres (driver tracking).
  ///
  /// With [background] the updates keep coming while the app is not on screen:
  /// Android runs a foreground service with a persistent notification, iOS keeps
  /// location updates alive (needs the "location" background mode in Info.plist).
  static Stream<Position> stream({int distanceFilter = 10, bool background = false, String title = '', String text = ''}) {
    LocationSettings settings = LocationSettings(accuracy: LocationAccuracy.high, distanceFilter: distanceFilter);
    if (background && !kIsWeb) {
      if (defaultTargetPlatform == TargetPlatform.android) {
        settings = AndroidSettings(
          accuracy: LocationAccuracy.high,
          distanceFilter: distanceFilter,
          intervalDuration: const Duration(seconds: 5),
          foregroundNotificationConfig: ForegroundNotificationConfig(
            notificationTitle: title,
            notificationText: text,
            notificationChannelName: 'Onlaýn iş',
            setOngoing: true,
            enableWakeLock: true,
          ),
        );
      } else if (defaultTargetPlatform == TargetPlatform.iOS) {
        settings = AppleSettings(
          accuracy: LocationAccuracy.high,
          distanceFilter: distanceFilter,
          activityType: ActivityType.automotiveNavigation,
          pauseLocationUpdatesAutomatically: false,
          allowBackgroundLocationUpdates: true,
          showBackgroundLocationIndicator: true,
        );
      }
    }
    return Geolocator.getPositionStream(locationSettings: settings);
  }

  static Future<void> openSettings() => Geolocator.openAppSettings();
}
