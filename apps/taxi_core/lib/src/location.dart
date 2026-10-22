import 'dart:async';

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
  static Stream<Position> stream({int distanceFilter = 10}) => Geolocator.getPositionStream(
        locationSettings: LocationSettings(accuracy: LocationAccuracy.high, distanceFilter: distanceFilter),
      );

  static Future<void> openSettings() => Geolocator.openAppSettings();
}
