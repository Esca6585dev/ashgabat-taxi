import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';

import '../models.dart';
import 'theme.dart';

LatLng ll(LatLngPoint p) => LatLng(p.lat, p.lng);

/// OpenStreetMap tiles — no API key, works for Turkmenistan.
/// Override with `--dart-define=TILE_URL=https://your-tiles/{z}/{x}/{y}.png` for a self-hosted server.
const String kTileUrl = String.fromEnvironment('TILE_URL', defaultValue: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png');

/// Map with the city tiles; children are layered on top (markers, route...).
class TaxiMap extends StatelessWidget {
  final MapController? controller;
  final LatLng center;
  final double zoom;
  final List<Widget> layers;
  final PositionCallback? onPositionChanged;
  final TapCallback? onTap;
  final VoidCallback? onMapReady;

  const TaxiMap({
    super.key,
    this.controller,
    required this.center,
    this.zoom = 15,
    this.layers = const [],
    this.onPositionChanged,
    this.onTap,
    this.onMapReady,
  });

  @override
  Widget build(BuildContext context) {
    return FlutterMap(
      mapController: controller,
      options: MapOptions(
        initialCenter: center,
        initialZoom: zoom,
        minZoom: 9,
        maxZoom: 19,
        onPositionChanged: onPositionChanged,
        onTap: onTap,
        onMapReady: onMapReady,
        interactionOptions: const InteractionOptions(flags: InteractiveFlag.all & ~InteractiveFlag.rotate),
      ),
      children: [
        TileLayer(urlTemplate: kTileUrl, userAgentPackageName: 'tm.ashgabattaxi', maxZoom: 19),
        ...layers,
        const SimpleAttributionWidget(source: Text('OpenStreetMap')),
      ],
    );
  }
}

/// Car icon rotated by heading.
Marker carMarker(LatLng p, {double heading = 0, Key? key, double size = 34}) => Marker(
      key: key,
      point: p,
      width: size,
      height: size,
      child: Transform.rotate(
        angle: heading * 3.1415926 / 180,
        child: Container(
          decoration: BoxDecoration(
            color: TaxiColors.yellow,
            shape: BoxShape.circle,
            border: Border.all(color: TaxiColors.dark, width: 2),
            boxShadow: const [BoxShadow(blurRadius: 4, color: Colors.black26)],
          ),
          child: const Icon(Icons.local_taxi, size: 18, color: TaxiColors.dark),
        ),
      ),
    );

Marker pointMarker(LatLng p, {required Color color, IconData icon = Icons.place, String? label}) => Marker(
      point: p,
      width: 40,
      height: 40,
      alignment: Alignment.topCenter,
      child: Icon(icon, color: color, size: 40, shadows: const [Shadow(blurRadius: 4, color: Colors.black38)]),
    );

Marker myLocationMarker(LatLng p) => Marker(
      point: p,
      width: 22,
      height: 22,
      child: Container(
        decoration: BoxDecoration(
          color: TaxiColors.blue,
          shape: BoxShape.circle,
          border: Border.all(color: Colors.white, width: 3),
          boxShadow: const [BoxShadow(blurRadius: 6, color: Colors.black26)],
        ),
      ),
    );

PolylineLayer routeLayer(List<LatLngPoint> pts) => PolylineLayer(polylines: [
      Polyline(points: pts.map(ll).toList(), strokeWidth: 5, color: TaxiColors.dark.withValues(alpha: 0.8)),
    ]);

/// The fixed pin in the screen centre used to pick a pickup point.
class CenterPin extends StatelessWidget {
  final String? label;
  const CenterPin({super.key, this.label});

  @override
  Widget build(BuildContext context) {
    return IgnorePointer(
      child: Center(
        child: Padding(
          padding: const EdgeInsets.only(bottom: 44),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            if (label != null)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
                margin: const EdgeInsets.only(bottom: 4),
                decoration: BoxDecoration(color: TaxiColors.dark, borderRadius: BorderRadius.circular(10)),
                child: Text(label!, style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600)),
              ),
            const Icon(Icons.location_on, size: 44, color: TaxiColors.dark, shadows: [Shadow(blurRadius: 4, color: Colors.black38)]),
          ]),
        ),
      ),
    );
  }
}

/// Fits the camera to all [points] (no-op for fewer than 2).
void fitPoints(MapController c, List<LatLngPoint> points, {EdgeInsets padding = const EdgeInsets.fromLTRB(48, 96, 48, 320)}) {
  final pts = points.where((p) => !p.isZero).map(ll).toList();
  if (pts.length < 2) {
    if (pts.length == 1) c.move(pts.first, 16);
    return;
  }
  c.fitCamera(CameraFit.coordinates(coordinates: pts, padding: padding, maxZoom: 17));
}
