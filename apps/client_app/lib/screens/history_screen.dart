import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

/// Past orders of the client with paging and per-order details.
class HistoryScreen extends StatefulWidget {
  final Session session;
  const HistoryScreen({super.key, required this.session});

  @override
  State<HistoryScreen> createState() => _HistoryScreenState();
}

class _HistoryScreenState extends State<HistoryScreen> {
  final orders = <Order>[];
  int page = 1;
  bool loading = false;
  bool done = false;

  @override
  void initState() {
    super.initState();
    _more();
  }

  Future<void> _reload() async {
    setState(() {
      orders.clear();
      page = 1;
      done = false;
    });
    await _more();
  }

  Future<void> _more() async {
    if (loading || done) return;
    setState(() => loading = true);
    final r = await guard(context, () => widget.session.api.myOrders(page: page));
    if (!mounted) return;
    setState(() {
      loading = false;
      if (r != null) {
        orders.addAll(r);
        page++;
        done = r.length < 30;
      } else {
        done = true;
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final cur = widget.session.config.currency;
    return Scaffold(
      appBar: AppBar(title: const Text('Syýahatlarym')),
      body: RefreshIndicator(
        onRefresh: _reload,
        child: orders.isEmpty && !loading
            ? ListView(children: const [
                SizedBox(height: 120),
                Icon(Icons.route_outlined, size: 64, color: TaxiColors.grey),
                SizedBox(height: 12),
                Text('Entek syýahat ýok', textAlign: TextAlign.center, style: TextStyle(color: TaxiColors.grey)),
              ])
            : ListView.separated(
                padding: const EdgeInsets.all(16),
                itemCount: orders.length + (done ? 0 : 1),
                separatorBuilder: (_, _) => const SizedBox(height: 10),
                itemBuilder: (_, i) {
                  if (i >= orders.length) {
                    _more();
                    return const Center(child: Padding(padding: EdgeInsets.all(16), child: CircularProgressIndicator()));
                  }
                  final o = orders[i];
                  return Card(
                    child: InkWell(
                      borderRadius: BorderRadius.circular(16),
                      onTap: () => _details(o),
                      child: Padding(
                        padding: const EdgeInsets.all(14),
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Row(children: [
                            Text(dateTime(o.createdAt), style: const TextStyle(color: TaxiColors.grey)),
                            const Spacer(),
                            StatusChip(o.status, delivery: o.isDelivery),
                          ]),
                          const SizedBox(height: 10),
                          RouteText(from: o.pickupAddress, to: o.dropoffAddress),
                          const SizedBox(height: 10),
                          Row(children: [
                            Text('${o.tariff?.name ?? ''} · ${km(o.distanceKm)}', style: const TextStyle(color: TaxiColors.grey)),
                            const Spacer(),
                            Text(money(o.price, cur), style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 16)),
                          ]),
                        ]),
                      ),
                    ),
                  );
                },
              ),
      ),
    );
  }

  void _details(Order o) {
    final cur = widget.session.config.currency;
    showModalBottomSheet<void>(
      context: context,
      showDragHandle: true,
      isScrollControlled: true,
      builder: (c) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 20),
          child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            Text('Sargyt #${o.id}', style: Theme.of(c).textTheme.titleLarge),
            const SizedBox(height: 4),
            Text(dateTime(o.createdAt), style: const TextStyle(color: TaxiColors.grey)),
            const SizedBox(height: 16),
            RouteText(from: o.pickupAddress, to: o.dropoffAddress),
            const Divider(height: 28),
            _row('Ýagdaýy', orderStatusText(o.status, delivery: o.isDelivery)),
            if (o.cancelReason.isNotEmpty) _row('Sebäbi', o.cancelReason),
            _row('Tarif', o.tariff?.name ?? ''),
            _row('Aralyk', km(o.distanceKm)),
            _row('Töleg', paymentText(o.paymentMethod)),
            if (o.discount > 0) _row('Arzanladyş', money(o.discount, cur)),
            _row('Bahasy', money(o.price, cur)),
            if (o.driver != null) _row('Sürüji', '${o.driver!.user?.displayName ?? ''} · ${o.driver!.carTitle} · ${o.driver!.carPlate}'),
            if (o.clientRating > 0) _row('Siziň bahaňyz', '★' * o.clientRating),
            const SizedBox(height: 16),
            if (o.status == OrderStatus.completed && o.clientRating == 0)
              FilledButton(
                onPressed: () async {
                  Navigator.pop(c);
                  final r = await askRating(context, 'Sürüjä baha beriň');
                  if (r != null && mounted) {
                    await guard(context, () => widget.session.api.rateDriver(o.id, r.$1, r.$2));
                    _reload();
                  }
                },
                child: const Text('Baha ber'),
              ),
            const SizedBox(height: 8),
            OutlinedButton.icon(
              onPressed: () {
                Navigator.pop(c);
                Navigator.push(context, MaterialPageRoute(builder: (_) => SupportScreen(session: widget.session, orderId: o.id)));
              },
              icon: const Icon(Icons.support_agent),
              label: const Text('Bu sargyt barada ýüz tut'),
            ),
          ]),
        ),
      ),
    );
  }

  Widget _row(String k, String v) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          SizedBox(width: 120, child: Text(k, style: const TextStyle(color: TaxiColors.grey))),
          Expanded(child: Text(v, style: const TextStyle(fontWeight: FontWeight.w600))),
        ]),
      );
}
