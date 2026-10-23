import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:taxi_core/taxi_core.dart';
import 'package:url_launcher/url_launcher.dart';

import '../driver_controller.dart';
import 'earnings_screen.dart';
import 'history_screen.dart';
import 'profile_screen.dart';

/// Work screen: map, online switch, incoming offer card and the current ride.
class DriverHome extends StatefulWidget {
  final Session session;
  final DriverController ctrl;
  const DriverHome({super.key, required this.session, required this.ctrl});

  @override
  State<DriverHome> createState() => _DriverHomeState();
}

class _DriverHomeState extends State<DriverHome> {
  final map = MapController();
  bool mapReady = false;
  bool follow = true;
  bool busy = false;
  StreamSubscription? _noticeSub;
  int? _fittedOrder;
  String? _fittedStatus;

  DriverController get c => widget.ctrl;
  String get cur => widget.session.config.currency;

  @override
  void initState() {
    super.initState();
    c.addListener(_onChange);
    _noticeSub = c.notices.listen(_onNotice);
  }

  @override
  void dispose() {
    c.removeListener(_onChange);
    _noticeSub?.cancel();
    super.dispose();
  }

  void _onChange() {
    if (!mounted || !mapReady) return;
    final o = c.order;
    if (o != null) {
      // re-frame once per order step: driver + pickup while going to the client, pickup + dropoff during the ride
      if (_fittedOrder != o.id || _fittedStatus != o.status) {
        _fittedOrder = o.id;
        _fittedStatus = o.status;
        fitPoints(map, [if (c.pos != null) c.pos!, if (o.status != OrderStatus.inProgress) o.pickup, o.dropoff]);
      }
    } else if (follow && c.pos != null) {
      map.move(ll(c.pos!), map.camera.zoom < 14 ? 16 : map.camera.zoom);
    }
  }

  void _onNotice(DriverNotice n) {
    if (!mounted) return;
    switch (n.kind) {
      case 'new':
        HapticFeedback.heavyImpact();
        SystemSound.play(SystemSoundType.alert);
      case 'cancelled':
        showDialog<void>(
          context: context,
          builder: (d) => AlertDialog(
            title: Text(n.text),
            content: n.order?.cancelReason.isNotEmpty == true ? Text('Sebäbi: ${n.order!.cancelReason}') : null,
            actions: [TextButton(onPressed: () => Navigator.pop(d), child: const Text('Bolýar'))],
          ),
        );
      case 'billing':
        _needTopUp(n.text);
      default:
        showMsg(context, n.text);
    }
  }

  Future<void> _run(Future<void> Function() f) async {
    if (busy) return;
    setState(() => busy = true);
    try {
      await f();
    } on ApiException catch (e) {
      // 402: no money for the plan -> offer a top-up instead of a bare error
      if (mounted) e.status == 402 ? _needTopUp(e.message) : showMsg(context, e.message, error: true);
    } catch (e) {
      if (mounted) showMsg(context, 'Ýalňyşlyk: $e', error: true);
    }
    if (mounted) setState(() => busy = false);
  }

  Future<void> _needTopUp(String why) async {
    final go = await showDialog<bool>(
      context: context,
      builder: (d) => AlertDialog(
        icon: const Icon(Icons.account_balance_wallet_outlined, size: 40),
        title: const Text('Balansy dolduryň'),
        content: Text(why),
        actions: [
          TextButton(onPressed: () => Navigator.pop(d, false), child: const Text('Soňra')),
          FilledButton(
            style: FilledButton.styleFrom(minimumSize: const Size(120, 44)),
            onPressed: () => Navigator.pop(d, true),
            child: const Text('Doldur'),
          ),
        ],
      ),
    );
    if (go == true && mounted) {
      await Navigator.push(context, MaterialPageRoute(builder: (_) => EarningsScreen(session: widget.session)));
      c.refreshProfile();
    }
  }

  Future<void> _toggleOnline() async {
    final v = !c.online;
    await _run(() => c.setOnline(v));
    if (mounted && c.online) {
      follow = true;
      _onChange();
    }
  }

  Future<void> _complete() async {
    if (!await confirm(context, 'Syýahaty tamamlamalymy?', ok: 'Tamamla', cancel: 'Yza')) return;
    if (!mounted) return;
    setState(() => busy = true);
    final o = await guard(context, c.complete);
    if (!mounted) return;
    setState(() => busy = false);
    if (o == null) return;
    await showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (d) => AlertDialog(
        title: const Text('Syýahat tamamlandy'),
        content: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(
            o.paymentMethod == 'cash' ? 'Müşderiden nagt alyň:' : 'Kart bilen tölendi:',
            style: const TextStyle(color: TaxiColors.grey),
          ),
          Text(money(o.finalPrice, cur), style: const TextStyle(fontSize: 34, fontWeight: FontWeight.w900)),
          const SizedBox(height: 8),
          Text('Komissiýa: ${money(o.commission, cur)}', style: const TextStyle(color: TaxiColors.grey)),
        ]),
        actions: [FilledButton(onPressed: () => Navigator.pop(d), child: const Text('Bolýar'))],
      ),
    );
    if (!mounted) return;
    final r = await askRating(context, 'Müşderä baha beriň', withComment: false);
    if (r != null && mounted) await guard(context, () => widget.session.api.rateClient(o.id, r.$1));
  }

  Future<void> _cancelRide() async {
    final reason = await askReason(context, 'Näme üçin ýatyrýarsyňyz?', [
      'Müşderi gelmedi',
      'Müşderi jogap bermeýär',
      'Awtoulag näsaz',
      'Salgy gaty uzak',
      'Başga sebäp',
    ]);
    if (reason == null) return;
    await _run(() => c.cancel(reason));
  }

  Future<void> _navigate(LatLngPoint p) async {
    final geo = Uri.parse('geo:${p.lat},${p.lng}?q=${p.lat},${p.lng}');
    try {
      if (await launchUrl(geo)) return;
    } catch (_) {}
    await launchUrl(
      Uri.parse('https://www.google.com/maps/dir/?api=1&destination=${p.lat},${p.lng}&travelmode=driving'),
      mode: LaunchMode.externalApplication,
    );
  }

  void _open(Widget page) {
    Navigator.pop(context);
    Navigator.push(context, MaterialPageRoute(builder: (_) => page));
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: c,
      builder: (context, _) {
        final o = c.order;
        final cfg = widget.session.config;
        return Scaffold(
          drawer: _drawer(),
          body: Stack(children: [
            TaxiMap(
              controller: map,
              center: c.pos == null ? ll(LatLngPoint(cfg.cityLat, cfg.cityLng)) : ll(c.pos!),
              zoom: 15,
              onMapReady: () {
                mapReady = true;
                WidgetsBinding.instance.addPostFrameCallback((_) => _onChange());
              },
              onPositionChanged: (cam, gesture) {
                if (gesture && follow) setState(() => follow = false);
              },
              layers: [
                if (o != null)
                  MarkerLayer(markers: [
                    pointMarker(ll(o.pickup), color: TaxiColors.green),
                    pointMarker(ll(o.dropoff), color: TaxiColors.red),
                  ]),
                if (o == null && c.offers.isNotEmpty) MarkerLayer(markers: [pointMarker(ll(c.offers.first.order.pickup), color: TaxiColors.green)]),
                if (c.pos != null) MarkerLayer(markers: [carMarker(ll(c.pos!), heading: c.heading, size: 42)]),
              ],
            ),
            SafeArea(
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Row(children: [
                  Builder(builder: (bc) => _round(Icons.menu, () => Scaffold.of(bc).openDrawer())),
                  const Spacer(),
                  _balanceChip(),
                  const Spacer(),
                  _round(follow ? Icons.navigation : Icons.navigation_outlined, () {
                    setState(() => follow = true);
                    if (c.pos != null) map.move(ll(c.pos!), 16);
                  }),
                ]),
              ),
            ),
            Align(alignment: Alignment.bottomCenter, child: _bottom()),
          ]),
        );
      },
    );
  }

  Widget _bottom() {
    final o = c.order;
    if (o != null) return _ridePanel(o);
    if (c.online && c.offers.isNotEmpty) return _offerCard(c.offers.first);
    return _panel(Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(children: [
        Icon(Icons.circle, size: 12, color: c.online ? TaxiColors.green : TaxiColors.grey),
        const SizedBox(width: 8),
        Text(c.online ? 'Onlaýn — sargyt garaşylýar' : 'Siz oflaýn', style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
        const Spacer(),
        if (c.online) const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2)),
      ]),
      const SizedBox(height: 14),
      FilledButton(
        key: const Key('onlineBtn'),
        style: c.online ? FilledButton.styleFrom(backgroundColor: TaxiColors.dark, foregroundColor: Colors.white) : null,
        onPressed: busy || c.switching ? null : _toggleOnline,
        child: Text(c.online ? 'Işi bes et' : 'Işe başla'),
      ),
    ]));
  }

  Widget _offerCard(OrderOffer f) {
    final o = f.order;
    final left = DriverController.offerSeconds - DateTime.now().difference(f.receivedAt).inSeconds;
    return _panel(Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(children: [
        Text(o.isDelivery ? 'Täze eltip bermek' : 'Täze sargyt', style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800)),
        const Spacer(),
        Text(money(o.price, cur), style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w900)),
      ]),
      const SizedBox(height: 4),
      Text(
        'Size çenli ${km(f.distanceKm)}${f.etaMin > 0 ? ' · ~${f.etaMin} min' : ''} · ýol ${km(o.distanceKm)} · ${o.tariff?.name ?? ''} · ${paymentText(o.paymentMethod)}',
        style: const TextStyle(color: TaxiColors.grey),
      ),
      const SizedBox(height: 12),
      if (o.scheduledAt != null) ...[
        Text('Bellenen wagt: ${whenText(o.scheduledAt!)}', style: const TextStyle(fontWeight: FontWeight.w700, color: TaxiColors.blue)),
        const SizedBox(height: 8),
      ],
      RouteText(from: o.pickupAddress, to: o.dropoffAddress),
      if (o.comment.isNotEmpty) ...[
        const SizedBox(height: 8),
        Text('«${o.comment}»', style: const TextStyle(fontStyle: FontStyle.italic)),
      ],
      const SizedBox(height: 12),
      LinearProgressIndicator(value: (left / DriverController.offerSeconds).clamp(0, 1), minHeight: 4, borderRadius: BorderRadius.circular(2)),
      const SizedBox(height: 12),
      Row(children: [
        Expanded(
          child: OutlinedButton(onPressed: busy ? null : () => c.decline(f), child: const Text('Ýok')),
        ),
        const SizedBox(width: 10),
        Expanded(
          flex: 2,
          child: FilledButton(
            key: const Key('acceptBtn'),
            onPressed: busy ? null : () => _run(() => c.accept(f)),
            child: Text('Kabul et · ${left.clamp(0, 99)}'),
          ),
        ),
      ]),
      if (c.offers.length > 1)
        Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Text('Ýene ${c.offers.length - 1} sargyt bar', textAlign: TextAlign.center, style: const TextStyle(color: TaxiColors.grey)),
        ),
    ]));
  }

  Widget _ridePanel(Order o) {
    final client = o.client;
    final (String title, String action, Future<void> Function() step, LatLngPoint target) = switch (o.status) {
      OrderStatus.accepted => ('Müşderä barýarsyňyz', 'Geldim', c.arrived, o.pickup),
      OrderStatus.arrived => ('Müşderi garaşylýar', 'Ýola çyk', c.start, o.dropoff),
      _ => (o.isDelivery ? 'Eltip berýärsiňiz' : 'Ýolda', 'Tamamla', () async {}, o.dropoff),
    };
    return _panel(Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(children: [
        Expanded(child: Text(title, style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800))),
        Text(money(o.price, cur), style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w900)),
      ]),
      Text('${o.tariff?.name ?? ''} · ${paymentText(o.paymentMethod)} · ${km(o.distanceKm)}', style: const TextStyle(color: TaxiColors.grey)),
      const Divider(height: 22),
      Row(children: [
        const CircleAvatar(backgroundColor: TaxiColors.yellow, child: Icon(Icons.person, color: TaxiColors.dark)),
        const SizedBox(width: 12),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(client?.displayName ?? 'Müşderi', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
            Text('★ ${(client?.rating ?? 5).toStringAsFixed(1)}', style: const TextStyle(color: TaxiColors.grey)),
          ]),
        ),
        if (client != null) IconButton.filledTonal(onPressed: () => callPhone(client.phone), icon: const Icon(Icons.phone)),
        if (o.isDelivery && o.recipientPhone.isNotEmpty)
          IconButton.filledTonal(tooltip: 'Alyjy', onPressed: () => callPhone(o.recipientPhone), icon: const Icon(Icons.inventory_2_outlined)),
      ]),
      const SizedBox(height: 12),
      RouteText(from: o.pickupAddress, to: o.dropoffAddress),
      if (o.comment.isNotEmpty) ...[
        const SizedBox(height: 8),
        Text('«${o.comment}»', style: const TextStyle(fontStyle: FontStyle.italic)),
      ],
      const SizedBox(height: 14),
      Row(children: [
        Expanded(
          child: OutlinedButton.icon(onPressed: () => _navigate(target), icon: const Icon(Icons.navigation_outlined), label: const Text('Ugur')),
        ),
        if (o.status != OrderStatus.inProgress) ...[
          const SizedBox(width: 10),
          Expanded(
            child: OutlinedButton(
              style: OutlinedButton.styleFrom(foregroundColor: TaxiColors.red),
              onPressed: busy ? null : _cancelRide,
              child: const Text('Ýatyr'),
            ),
          ),
        ],
      ]),
      const SizedBox(height: 10),
      FilledButton(
        key: const Key('stepBtn'),
        onPressed: busy ? null : (o.status == OrderStatus.inProgress ? _complete : () => _run(step)),
        child: Text(action),
      ),
    ]));
  }

  Widget _panel(Widget child) => Container(
        decoration: const BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.vertical(top: Radius.circular(22)),
          boxShadow: [BoxShadow(blurRadius: 16, color: Colors.black12)],
        ),
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 10),
        child: SafeArea(top: false, child: child),
      );

  Widget _round(IconData i, VoidCallback onTap) => Material(
        color: Colors.white,
        shape: const CircleBorder(),
        elevation: 3,
        child: InkWell(customBorder: const CircleBorder(), onTap: onTap, child: Padding(padding: const EdgeInsets.all(12), child: Icon(i))),
      );

  Widget _balanceChip() {
    final b = c.driver?.balance ?? 0;
    return Material(
      color: TaxiColors.dark,
      borderRadius: BorderRadius.circular(24),
      elevation: 3,
      child: InkWell(
        borderRadius: BorderRadius.circular(24),
        onTap: () => Navigator.push(context, MaterialPageRoute(builder: (_) => EarningsScreen(session: widget.session))),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
          child: Text(
            'Balans: ${money(b, cur)}',
            style: TextStyle(color: b < 0 ? const Color(0xFFFF8A80) : TaxiColors.yellow, fontWeight: FontWeight.w800),
          ),
        ),
      ),
    );
  }

  Drawer _drawer() {
    final u = widget.session.user;
    final d = c.driver;
    return Drawer(
      child: SafeArea(
        child: Column(children: [
          ListTile(
            contentPadding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
            leading: const CircleAvatar(backgroundColor: TaxiColors.yellow, child: Icon(Icons.drive_eta, color: TaxiColors.dark)),
            title: Text(u?.displayName ?? '', style: const TextStyle(fontWeight: FontWeight.w700)),
            subtitle: Text('${d?.carPlate ?? ''} · ★ ${(u?.rating ?? 5).toStringAsFixed(1)} · ${d?.totalRides ?? 0} syýahat'),
          ),
          const Divider(),
          ListTile(
            leading: const Icon(Icons.account_balance_wallet_outlined),
            title: const Text('Gazanç we balans'),
            onTap: () => _open(EarningsScreen(session: widget.session)),
          ),
          ListTile(leading: const Icon(Icons.history), title: const Text('Sargytlar taryhy'), onTap: () => _open(DriverHistoryScreen(session: widget.session))),
          ListTile(
            leading: const Icon(Icons.directions_car_outlined),
            title: const Text('Profil we awtoulag'),
            onTap: () => _open(DriverProfileScreen(session: widget.session, ctrl: c)),
          ),
          ListTile(leading: const Icon(Icons.support_agent), title: const Text('Goldaw gullugy'), onTap: () => _open(SupportScreen(session: widget.session))),
          const Spacer(),
          const Padding(padding: EdgeInsets.all(16), child: Text('Aşgabat Taxi Sürüji · 1.0', style: TextStyle(color: TaxiColors.grey))),
        ]),
      ),
    );
  }
}
