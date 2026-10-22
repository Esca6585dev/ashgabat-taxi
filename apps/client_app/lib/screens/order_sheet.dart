import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:taxi_core/taxi_core.dart';

/// Bottom sheet of the confirm step: route, tariff cards with prices, payment, promo, comment.
class OrderSheet extends StatefulWidget {
  final Session session;
  final Place pickup;
  final Place dropoff;
  final VoidCallback onChangePickup;
  final VoidCallback onChangeDropoff;
  final void Function(Estimate) onEstimate;
  final void Function(Order) onOrdered;

  const OrderSheet({
    super.key,
    required this.session,
    required this.pickup,
    required this.dropoff,
    required this.onChangePickup,
    required this.onChangeDropoff,
    required this.onEstimate,
    required this.onOrdered,
  });

  @override
  State<OrderSheet> createState() => _OrderSheetState();
}

class _OrderSheetState extends State<OrderSheet> {
  Estimate? est;
  String? error;
  int selected = 0;
  String payment = 'cash';
  PromoResult? promo;
  String comment = '';
  String recipientPhone = '';
  bool busy = false;

  ApiClient get api => widget.session.api;
  String get cur => widget.session.config.currency;

  @override
  void initState() {
    super.initState();
    _estimate();
  }

  @override
  void didUpdateWidget(covariant OrderSheet old) {
    super.didUpdateWidget(old);
    if (old.pickup.lat != widget.pickup.lat ||
        old.pickup.lng != widget.pickup.lng ||
        old.dropoff.lat != widget.dropoff.lat ||
        old.dropoff.lng != widget.dropoff.lng) {
      _estimate();
    }
  }

  Future<void> _estimate() async {
    setState(() {
      est = null;
      error = null;
    });
    try {
      final e = await api.estimate(widget.pickup, widget.dropoff);
      if (!mounted) return;
      setState(() {
        est = e;
        if (selected >= e.options.length) selected = 0;
      });
      widget.onEstimate(e);
    } on ApiException catch (e) {
      if (mounted) setState(() => error = e.message);
    }
  }

  TariffOption? get _opt => est == null || est!.options.isEmpty ? null : est!.options[selected];
  bool get _isDelivery => _opt?.tariff.code == 'delivery';

  /// The server refuses rides shorter than 200 m.
  bool get _tooClose => est != null && est!.distanceKm < 0.2;

  double _discount(double price) {
    return promo?.discountFor(price) ?? 0;
  }

  Future<void> _askPromo() async {
    final ctrl = TextEditingController(text: promo?.code ?? '');
    final code = await showDialog<String>(
      context: context,
      builder: (c) => AlertDialog(
        title: const Text('Promokod'),
        content: TextField(
          controller: ctrl,
          autofocus: true,
          textCapitalization: TextCapitalization.characters,
          decoration: const InputDecoration(hintText: 'SALAM10'),
        ),
        actions: [
          if (promo != null) TextButton(onPressed: () => Navigator.pop(c, ''), child: const Text('Aýyr')),
          TextButton(onPressed: () => Navigator.pop(c), child: const Text('Ýap')),
          FilledButton(
            style: FilledButton.styleFrom(minimumSize: const Size(90, 44)),
            onPressed: () => Navigator.pop(c, ctrl.text.trim()),
            child: const Text('Ulan'),
          ),
        ],
      ),
    );
    ctrl.dispose();
    if (code == null || !mounted) return;
    if (code.isEmpty) {
      setState(() => promo = null);
      return;
    }
    final r = await guard(context, () => api.checkPromo(code, _opt?.price ?? 0));
    if (r != null && mounted) {
      setState(() => promo = r);
      showMsg(context, 'Promokod kabul edildi: −${money(r.discount, cur)}');
    }
  }

  Future<void> _askComment() async {
    final c1 = TextEditingController(text: comment);
    final c2 = TextEditingController(text: recipientPhone);
    final ok = await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (c) => Padding(
        padding: EdgeInsets.fromLTRB(20, 0, 20, MediaQuery.of(c).viewInsets.bottom + 20),
        child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(_isDelivery ? 'Eltip bermek barada' : 'Sürüjä teswir', style: Theme.of(c).textTheme.titleLarge),
          const SizedBox(height: 12),
          TextField(
            controller: c1,
            maxLines: 3,
            decoration: InputDecoration(hintText: _isDelivery ? 'Näme eltmeli, girelge, gat...' : 'Meselem: 3-nji girelgäniň öňünde'),
          ),
          if (_isDelivery) ...[
            const SizedBox(height: 12),
            TextField(
              controller: c2,
              keyboardType: TextInputType.phone,
              inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9 +\-]'))],
              decoration: const InputDecoration(labelText: 'Alyjynyň telefony', prefixText: '+993 '),
            ),
          ],
          const SizedBox(height: 16),
          FilledButton(onPressed: () => Navigator.pop(c, true), child: const Text('Ýatda sakla')),
        ]),
      ),
    );
    if (ok == true && mounted) {
      setState(() {
        comment = c1.text.trim();
        recipientPhone = c2.text.trim();
      });
    }
    c1.dispose();
    c2.dispose();
  }

  Future<void> _order() async {
    final opt = _opt;
    if (opt == null) return;
    if (_isDelivery && recipientPhone.isNotEmpty && !validPhone(normalizePhone(recipientPhone))) {
      showMsg(context, 'Alyjynyň telefon belgisi nädogry', error: true);
      return;
    }
    setState(() => busy = true);
    try {
      final o = await api.createOrder(
        tariffId: opt.tariff.id,
        pickup: widget.pickup,
        dropoff: widget.dropoff,
        paymentMethod: payment,
        promoCode: promo?.code,
        comment: comment,
        recipientPhone: recipientPhone,
      );
      widget.onOrdered(o);
    } on ApiException catch (e) {
      if (!mounted) return;
      if (e.status == 409 && e.data is Map) {
        // an order is already running (e.g. created from another phone) -> show it
        widget.onOrdered(Order.fromJson((e.data as Map).cast<String, dynamic>()));
      } else {
        showMsg(context, e.message, error: true);
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final e = est;
    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.vertical(top: Radius.circular(22)),
        boxShadow: [BoxShadow(blurRadius: 16, color: Colors.black12)],
      ),
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 10),
      child: SafeArea(
        top: false,
        child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          InkWell(onTap: widget.onChangePickup, child: _addr(TaxiColors.green, widget.pickup.title)),
          InkWell(onTap: widget.onChangeDropoff, child: _addr(TaxiColors.red, widget.dropoff.title)),
          const SizedBox(height: 10),
          if (error != null) ...[
            Text(error!, style: const TextStyle(color: TaxiColors.red), textAlign: TextAlign.center),
            TextButton(onPressed: _estimate, child: const Text('Täzeden synanyş')),
          ] else if (e == null)
            const Padding(padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator()))
          else ...[
            Text(
              '${km(e.distanceKm)} · ~${e.durationMin} min'
              '${e.driversNearby > 0 ? ' · sürüji ~${e.pickupEtaMin} min' : ' · golaýda boş sürüji ýok'}'
              '${e.isNight ? ' · gijeki tarif' : ''}',
              style: t.bodyMedium?.copyWith(color: TaxiColors.grey),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 10),
            SizedBox(
              height: 112,
              child: ListView.separated(
                scrollDirection: Axis.horizontal,
                itemCount: e.options.length,
                separatorBuilder: (_, _) => const SizedBox(width: 10),
                itemBuilder: (_, i) => _tariffCard(e.options[i], i == selected, () => setState(() => selected = i)),
              ),
            ),
            const SizedBox(height: 8),
            Row(children: [
              _chip(payment == 'cash' ? Icons.payments_outlined : Icons.credit_card, paymentText(payment), () {
                setState(() => payment = payment == 'cash' ? 'card' : 'cash');
              }),
              const SizedBox(width: 8),
              _chip(Icons.percent, promo == null ? 'Promokod' : promo!.code, _askPromo, active: promo != null),
              const SizedBox(width: 8),
              _chip(Icons.chat_bubble_outline, comment.isEmpty ? 'Teswir' : 'Teswir ✓', _askComment, active: comment.isNotEmpty),
            ]),
            const SizedBox(height: 10),
            FilledButton(
              key: const Key('orderBtn'),
              onPressed: busy || _opt == null || _tooClose ? null : _order,
              child: busy
                  ? const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2.5))
                  : Text(_opt == null
                      ? 'Tarif ýok'
                      : _tooClose
                          ? 'Barmaly ýer gaty ýakyn'
                          : 'Sargyt et · ${money(_opt!.price - _discount(_opt!.price), cur)}'),
            ),
          ],
        ]),
      ),
    );
  }

  Widget _addr(Color c, String s) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: Row(children: [
          Icon(Icons.circle, size: 11, color: c),
          const SizedBox(width: 12),
          Expanded(child: Text(s.isEmpty ? 'Kartadaky nokat' : s, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 16))),
          const Icon(Icons.chevron_right, color: TaxiColors.grey, size: 20),
        ]),
      );

  Widget _tariffCard(TariffOption o, bool sel, VoidCallback onTap) {
    final d = _discount(o.price);
    return GestureDetector(
      onTap: onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 150),
        width: 118,
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: sel ? TaxiColors.yellow.withValues(alpha: 0.18) : const Color(0xFFF4F4F6),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: sel ? TaxiColors.yellow : Colors.transparent, width: 2),
        ),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Icon(_tariffIcon(o.tariff), size: 28, color: TaxiColors.dark),
          const Spacer(),
          Text(o.tariff.name, style: const TextStyle(fontWeight: FontWeight.w700), maxLines: 1, overflow: TextOverflow.ellipsis),
          if (d > 0)
            Text(money(o.price, cur), style: const TextStyle(fontSize: 12, color: TaxiColors.grey, decoration: TextDecoration.lineThrough)),
          Text(money(o.price - d, cur), style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w800)),
        ]),
      ),
    );
  }

  Widget _chip(IconData i, String label, VoidCallback onTap, {bool active = false}) => Expanded(
        child: Material(
          color: active ? TaxiColors.yellow.withValues(alpha: 0.2) : const Color(0xFFF4F4F6),
          borderRadius: BorderRadius.circular(12),
          child: InkWell(
            borderRadius: BorderRadius.circular(12),
            onTap: onTap,
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 6),
              child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                Icon(i, size: 18),
                const SizedBox(width: 6),
                Flexible(child: Text(label, maxLines: 1, overflow: TextOverflow.ellipsis)),
              ]),
            ),
          ),
        ),
      );
}

IconData _tariffIcon(Tariff t) => switch (t.icon.isNotEmpty ? t.icon : t.code) {
      'comfort' => Icons.directions_car_filled,
      'business' => Icons.airport_shuttle,
      'minivan' => Icons.airport_shuttle_outlined,
      'delivery' => Icons.inventory_2_outlined,
      _ => Icons.local_taxi,
    };
