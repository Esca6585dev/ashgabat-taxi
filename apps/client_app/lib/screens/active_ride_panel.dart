import 'dart:async';

import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

import '../ride_controller.dart';

/// Bottom panel while an order is running: searching spinner, then driver + car + actions.
class ActiveRidePanel extends StatefulWidget {
  final Session session;
  final RideController ride;
  const ActiveRidePanel({super.key, required this.session, required this.ride});

  @override
  State<ActiveRidePanel> createState() => _ActiveRidePanelState();
}

class _ActiveRidePanelState extends State<ActiveRidePanel> {
  Timer? _tick;
  bool busy = false;

  @override
  void initState() {
    super.initState();
    _tick = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    _tick?.cancel();
    super.dispose();
  }

  String get cur => widget.session.config.currency;

  Future<void> _cancel() async {
    final reason = await askReason(context, 'Näme üçin ýatyrýarsyňyz?', [
      'Meýilnama üýtgedi',
      'Sürüji gaty uzakda',
      'Sürüji gelmedi',
      'Ýalňyş salgy görkezdim',
      'Başga sebäp',
    ]);
    if (reason == null || !mounted) return;
    setState(() => busy = true);
    await guard(context, () => widget.ride.cancel(reason));
    if (mounted) setState(() => busy = false);
  }

  String _elapsed(DateTime? from) {
    if (from == null) return '';
    final d = DateTime.now().difference(from);
    if (d.isNegative) return '0:00';
    return '${d.inMinutes}:${two(d.inSeconds % 60)}';
  }

  @override
  Widget build(BuildContext context) {
    final o = widget.ride.order!;
    final t = Theme.of(context).textTheme;
    final d = o.driver;

    String title;
    String? sub;
    switch (o.status) {
      case OrderStatus.searching:
        title = 'Sürüji gözlenýär...';
        sub = _elapsed(o.createdAt);
      case OrderStatus.accepted:
        title = 'Sürüji size gelýär';
        sub = 'Kabul edildi ${_elapsed(o.acceptedAt)} öň';
      case OrderStatus.arrived:
        final free = o.tariff?.freeWaitMin ?? 0;
        title = 'Sürüji sizi garaşýar';
        sub = free > 0 ? 'Mugt garaşma $free min · ${_elapsed(o.arrivedAt)}' : _elapsed(o.arrivedAt);
      default:
        title = o.isDelivery ? 'Eltilýär' : 'Ýolda';
        sub = 'Ugur: ${o.dropoffAddress}';
    }

    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.vertical(top: Radius.circular(22)),
        boxShadow: [BoxShadow(blurRadius: 16, color: Colors.black12)],
      ),
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 10),
      child: SafeArea(
        top: false,
        child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Row(children: [
            if (o.status == OrderStatus.searching)
              const Padding(padding: EdgeInsets.only(right: 12), child: SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 3))),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(title, style: t.titleLarge?.copyWith(fontWeight: FontWeight.w800)),
                if (sub.isNotEmpty) Text(sub, style: t.bodyMedium?.copyWith(color: TaxiColors.grey), maxLines: 1, overflow: TextOverflow.ellipsis),
              ]),
            ),
            Text(money(o.price, cur), style: t.titleLarge?.copyWith(fontWeight: FontWeight.w800)),
          ]),
          if (d != null) ...[
            const Divider(height: 24),
            Row(children: [
              const CircleAvatar(radius: 24, backgroundColor: TaxiColors.yellow, child: Icon(Icons.person, color: TaxiColors.dark)),
              const SizedBox(width: 12),
              Expanded(
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text(d.user?.displayName ?? 'Sürüji', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
                  Text('★ ${(d.user?.rating ?? 5).toStringAsFixed(1)} · ${d.carTitle}', style: const TextStyle(color: TaxiColors.grey)),
                ]),
              ),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                decoration: BoxDecoration(border: Border.all(color: TaxiColors.dark, width: 1.5), borderRadius: BorderRadius.circular(8)),
                child: Text(d.carPlate, style: const TextStyle(fontWeight: FontWeight.w800, letterSpacing: 1)),
              ),
            ]),
          ],
          const SizedBox(height: 12),
          RouteText(from: o.pickupAddress, to: o.dropoffAddress),
          const SizedBox(height: 6),
          Text('${o.tariff?.name ?? ''} · ${paymentText(o.paymentMethod)}${o.discount > 0 ? ' · arzanladyş ${money(o.discount, cur)}' : ''}',
              style: const TextStyle(color: TaxiColors.grey)),
          const SizedBox(height: 12),
          Row(children: [
            if (d?.user != null)
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: () => callPhone(d!.user!.phone),
                  icon: const Icon(Icons.phone),
                  label: const Text('Jaň et'),
                ),
              ),
            if (d?.user != null && o.status != OrderStatus.inProgress) const SizedBox(width: 10),
            if (o.status != OrderStatus.inProgress)
              Expanded(
                child: OutlinedButton.icon(
                  key: const Key('cancelBtn'),
                  style: OutlinedButton.styleFrom(foregroundColor: TaxiColors.red),
                  onPressed: busy ? null : _cancel,
                  icon: const Icon(Icons.close),
                  label: const Text('Ýatyr'),
                ),
              ),
            if (o.status == OrderStatus.inProgress)
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: () => Navigator.push(
                    context,
                    MaterialPageRoute(builder: (_) => SupportScreen(session: widget.session, orderId: o.id)),
                  ),
                  icon: const Icon(Icons.support_agent),
                  label: const Text('Goldaw'),
                ),
              ),
          ]),
        ]),
      ),
    );
  }
}
