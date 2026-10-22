import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';
import 'package:taxi_core/taxi_core.dart';

import '../ride_controller.dart';
import 'active_ride_panel.dart';
import 'addresses_screen.dart';
import 'history_screen.dart';
import 'order_sheet.dart';
import 'profile_screen.dart';
import 'search_screen.dart';

/// Main screen: map + pickup pin, destination choice, tariff sheet and the active ride.
class HomeScreen extends StatefulWidget {
  final Session session;
  final RideController ride;
  const HomeScreen({super.key, required this.session, required this.ride});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  final map = MapController();
  bool mapReady = false;
  LatLngPoint? me;
  Place? pickup;
  Place? dropoff;
  bool resolving = false;
  bool pickupLocked = false; // true when pickup was chosen from search, not from the pin
  List<NearbyCar> cars = [];
  List<LatLngPoint> route = [];
  List<SavedAddress> saved = [];
  Timer? _geoDebounce;
  Timer? _carsTimer;
  StreamSubscription? _noticeSub;

  Session get s => widget.session;
  RideController get ride => widget.ride;

  @override
  void initState() {
    super.initState();
    ride.addListener(_onRide);
    _noticeSub = ride.notices.listen(_onNotice);
    _carsTimer = Timer.periodic(const Duration(seconds: 10), (_) => _loadCars());
    _locate(initial: true);
    _loadSaved();
  }

  @override
  void dispose() {
    ride.removeListener(_onRide);
    ride.dispose();
    _noticeSub?.cancel();
    _geoDebounce?.cancel();
    _carsTimer?.cancel();
    super.dispose();
  }

  LatLng get _center => LatLng(s.config.cityLat, s.config.cityLng);

  Future<void> _locate({bool initial = false}) async {
    final p = await LocationService.currentPoint();
    if (!mounted) return;
    if (p == null) {
      if (!initial) {
        final why = await LocationService.ensurePermission();
        if (mounted) showMsg(context, why ?? 'Ýerleşişi kesgitläp bolmady', error: true);
      }
      if (initial) _pinMoved(LatLngPoint(_center.latitude, _center.longitude));
      return;
    }
    setState(() => me = p);
    if (mapReady && !ride.hasOrder && dropoff == null) map.move(ll(p), 16);
    if (!ride.hasOrder && dropoff == null) {
      pickupLocked = false;
      _pinMoved(p);
    }
  }

  Future<void> _loadSaved() async {
    try {
      final a = await s.api.addresses();
      if (mounted) setState(() => saved = a);
    } catch (_) {}
  }

  Future<void> _loadCars() async {
    if (ride.hasOrder || !mounted) return;
    final c = pickup?.point ?? me;
    if (c == null) return;
    try {
      final r = await s.api.nearbyDrivers(c.lat, c.lng);
      if (mounted) setState(() => cars = r);
    } catch (_) {}
  }

  void _onRide() {
    if (!mounted) return;
    setState(() {});
    final o = ride.order;
    if (o != null && mapReady) {
      fitPoints(map, [if (ride.driverPos != null && o.status != OrderStatus.inProgress) ride.driverPos!, o.pickup, o.dropoff]);
    }
  }

  Future<void> _onNotice(RideNotice n) async {
    if (!mounted) return;
    _setDropoff(null);
    pickupLocked = false;
    switch (n.kind) {
      case 'completed':
        final r = await askRating(context, 'Syýahat tamamlandy · ${money(n.order.price, s.config.currency)}\nSürüjä baha beriň');
        if (r != null && mounted) {
          await guard(context, () => s.api.rateDriver(n.order.id, r.$1, r.$2));
          if (mounted) showMsg(context, 'Sag boluň! Bahaňyz kabul edildi.');
        }
      case 'cancelled':
        if (n.order.cancelledBy == 'client') return;
        final why = switch (n.order.cancelledBy) {
          'system' => 'Golaýda boş sürüji tapylmady. Biraz soňra täzeden synanyşyň.',
          'admin' => 'Sargydy operator ýatyrdy. ${n.reason}',
          _ => n.reason,
        };
        await showDialog<void>(
          context: context,
          builder: (c) => AlertDialog(
            title: const Text('Sargyt ýatyryldy'),
            content: why.trim().isEmpty ? null : Text(why),
            actions: [TextButton(onPressed: () => Navigator.pop(c), child: const Text('Bolýar'))],
          ),
        );
      case 'requeued':
        showMsg(context, 'Sürüji sargydy ýatyrdy. Täze sürüji gözlenýär...');
    }
    _locate();
  }

  void _pinMoved(LatLngPoint p) {
    _geoDebounce?.cancel();
    setState(() {
      resolving = true;
      pickup = Place(name: pickup?.name ?? '', lat: p.lat, lng: p.lng);
    });
    _geoDebounce = Timer(const Duration(milliseconds: 700), () async {
      Place? r;
      try {
        r = await s.api.reverse(p.lat, p.lng);
      } catch (_) {}
      if (!mounted || pickupLocked) return;
      setState(() {
        resolving = false;
        pickup = Place(name: r?.title ?? '', lat: p.lat, lng: p.lng);
      });
      _loadCars();
    });
  }

  Future<void> _chooseDropoff() async {
    final p = await Navigator.push<Place>(
      context,
      MaterialPageRoute(builder: (_) => SearchScreen(session: s, title: 'Nirä barýarsyňyz?', saved: saved, near: pickup?.point ?? me)),
    );
    if (p == null || !mounted) return;
    _setDropoff(p);
    _loadSaved();
  }

  /// Switches to the confirm step: straight line first, the real route arrives with the estimate.
  void _setDropoff(Place? p) {
    setState(() {
      dropoff = p;
      route = p == null || pickup == null ? [] : [pickup!.point, p.point];
    });
    if (route.isNotEmpty) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && mapReady) fitPoints(map, route);
      });
    }
  }

  Future<void> _choosePickup() async {
    final p = await Navigator.push<Place>(
      context,
      MaterialPageRoute(builder: (_) => SearchScreen(session: s, title: 'Nireden?', saved: saved, near: me)),
    );
    if (p == null || !mounted) return;
    setState(() {
      pickupLocked = true;
      pickup = p;
      resolving = false;
    });
    if (dropoff == null) {
      map.move(ll(p.point), 16);
    } else {
      _setDropoff(dropoff);
    }
  }

  void _openDrawerPage(Widget page) {
    Navigator.pop(context);
    Navigator.push(context, MaterialPageRoute(builder: (_) => page)).then((_) => _loadSaved());
  }

  @override
  Widget build(BuildContext context) {
    final o = ride.order;
    final picking = o == null && dropoff == null;
    final layers = <Widget>[];
    if (o != null) {
      layers.add(MarkerLayer(markers: [
        pointMarker(ll(o.pickup), color: TaxiColors.green),
        pointMarker(ll(o.dropoff), color: TaxiColors.red),
        if (ride.driverPos != null) carMarker(ll(ride.driverPos!), heading: ride.driverHeading, size: 40),
      ]));
    } else {
      layers.add(MarkerLayer(markers: [for (final c in cars) carMarker(LatLng(c.lat, c.lng), heading: c.heading)]));
      if (dropoff != null && pickup != null) {
        layers.add(MarkerLayer(markers: [
          pointMarker(ll(pickup!.point), color: TaxiColors.green),
          pointMarker(ll(dropoff!.point), color: TaxiColors.red),
        ]));
      }
    }
    if (me != null) layers.add(MarkerLayer(markers: [myLocationMarker(ll(me!))]));

    return Scaffold(
      drawer: _drawer(),
      body: Stack(children: [
        TaxiMap(
          controller: map,
          center: _center,
          zoom: 14,
          layers: [
            if (dropoff != null && o == null && route.length >= 2) routeLayer(route),
            ...layers,
          ],
          onMapReady: () {
            mapReady = true;
            final o = ride.order;
            if (o != null) {
              _onRide();
            } else if (me != null && dropoff == null) {
              map.move(ll(me!), 16);
            }
          },
          onPositionChanged: (camera, hasGesture) {
            if (picking && hasGesture) {
              pickupLocked = false;
              _pinMoved(LatLngPoint(camera.center.latitude, camera.center.longitude));
            }
          },
        ),
        if (picking) CenterPin(label: resolving ? '...' : null),
        SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Row(children: [
              Builder(
                builder: (c) => _RoundButton(
                  icon: dropoff != null && o == null ? Icons.arrow_back : Icons.menu,
                  onTap: () {
                    if (dropoff != null && o == null) {
                      _setDropoff(null);
                      if (pickup != null) map.move(ll(pickup!.point), 16);
                    } else {
                      Scaffold.of(c).openDrawer();
                    }
                  },
                ),
              ),
              const Spacer(),
              if (o == null) _RoundButton(icon: Icons.my_location, onTap: _locate),
            ]),
          ),
        ),
        Align(
          alignment: Alignment.bottomCenter,
          child: o != null
              ? ActiveRidePanel(session: s, ride: ride)
              : dropoff == null
                  ? _pickPanel()
                  : OrderSheet(
                      session: s,
                      pickup: pickup!,
                      dropoff: dropoff!,
                      onChangePickup: _choosePickup,
                      onChangeDropoff: _chooseDropoff,
                      onEstimate: (e) {
                        if (e.polyline.length >= 2) setState(() => route = e.polyline);
                      },
                      onOrdered: (order) {
                        ride.started(order);
                        _setDropoff(null);
                      },
                    ),
        ),
      ]),
    );
  }

  Widget _pickPanel() {
    final t = Theme.of(context).textTheme;
    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.vertical(top: Radius.circular(22)),
        boxShadow: [BoxShadow(blurRadius: 16, color: Colors.black12)],
      ),
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 12),
      child: SafeArea(
        top: false,
        child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          InkWell(
            onTap: _choosePickup,
            borderRadius: BorderRadius.circular(12),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Row(children: [
                const Icon(Icons.circle, size: 12, color: TaxiColors.green),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    resolving ? 'Salgy kesgitlenýär...' : (pickup?.title.isNotEmpty == true ? pickup!.title : 'Kartadaky nokat'),
                    style: t.titleMedium?.copyWith(fontWeight: FontWeight.w700),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                const Icon(Icons.edit_location_alt_outlined, color: TaxiColors.grey),
              ]),
            ),
          ),
          const SizedBox(height: 10),
          Material(
            color: const Color(0xFFF0F0F2),
            borderRadius: BorderRadius.circular(14),
            child: InkWell(
              key: const Key('whereTo'),
              borderRadius: BorderRadius.circular(14),
              onTap: pickup == null ? null : _chooseDropoff,
              child: const Padding(
                padding: EdgeInsets.all(16),
                child: Row(children: [
                  Icon(Icons.search, color: TaxiColors.dark),
                  SizedBox(width: 12),
                  Text('Nirä barýarsyňyz?', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
                ]),
              ),
            ),
          ),
          if (saved.isNotEmpty) ...[
            const SizedBox(height: 10),
            SizedBox(
              height: 40,
              child: ListView(scrollDirection: Axis.horizontal, children: [
                for (final a in saved)
                  Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: ActionChip(
                      avatar: Icon(_savedIcon(a.title), size: 18),
                      label: Text(a.title),
                      onPressed: () => _setDropoff(a.place),
                    ),
                  ),
              ]),
            ),
          ],
          if (cars.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text('Golaýda ${cars.length} sürüji bar', style: t.bodySmall?.copyWith(color: TaxiColors.grey), textAlign: TextAlign.center),
          ],
        ]),
      ),
    );
  }

  Drawer _drawer() {
    final u = s.user;
    return Drawer(
      child: SafeArea(
        child: Column(children: [
          ListTile(
            contentPadding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
            leading: const CircleAvatar(backgroundColor: TaxiColors.yellow, child: Icon(Icons.person, color: TaxiColors.dark)),
            title: Text(u?.displayName ?? '', style: const TextStyle(fontWeight: FontWeight.w700)),
            subtitle: Text('${prettyPhone(u?.phone ?? '')} · ★ ${(u?.rating ?? 5).toStringAsFixed(1)}'),
            onTap: () => _openDrawerPage(ProfileScreen(session: s)),
          ),
          const Divider(),
          ListTile(leading: const Icon(Icons.history), title: const Text('Syýahatlarym'), onTap: () => _openDrawerPage(HistoryScreen(session: s))),
          ListTile(leading: const Icon(Icons.bookmark_outline), title: const Text('Salgylarym'), onTap: () => _openDrawerPage(AddressesScreen(session: s))),
          ListTile(leading: const Icon(Icons.support_agent), title: const Text('Goldaw gullugy'), onTap: () => _openDrawerPage(SupportScreen(session: s))),
          ListTile(leading: const Icon(Icons.person_outline), title: const Text('Profil'), onTap: () => _openDrawerPage(ProfileScreen(session: s))),
          const Spacer(),
          const Padding(padding: EdgeInsets.all(16), child: Text('Aşgabat Taxi · 1.0', style: TextStyle(color: TaxiColors.grey))),
        ]),
      ),
    );
  }
}

IconData _savedIcon(String title) {
  final t = title.toLowerCase();
  if (t.contains('öý') || t.contains('oy') || t.contains('home')) return Icons.home_outlined;
  if (t.contains('iş') || t.contains('is') || t.contains('work')) return Icons.work_outline;
  return Icons.place_outlined;
}

class _RoundButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback onTap;
  const _RoundButton({required this.icon, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.white,
      shape: const CircleBorder(),
      elevation: 3,
      child: InkWell(customBorder: const CircleBorder(), onTap: onTap, child: Padding(padding: const EdgeInsets.all(12), child: Icon(icon))),
    );
  }
}
