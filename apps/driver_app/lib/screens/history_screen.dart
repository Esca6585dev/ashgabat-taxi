import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

/// Completed and cancelled orders of the driver.
class DriverHistoryScreen extends StatefulWidget {
  final Session session;
  const DriverHistoryScreen({super.key, required this.session});

  @override
  State<DriverHistoryScreen> createState() => _DriverHistoryScreenState();
}

class _DriverHistoryScreenState extends State<DriverHistoryScreen> {
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
    final r = await guard(context, () => widget.session.api.driverOrders(page: page));
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
      appBar: AppBar(title: const Text('Sargytlar taryhy')),
      body: RefreshIndicator(
        onRefresh: _reload,
        child: orders.isEmpty && !loading
            ? ListView(children: const [
                SizedBox(height: 120),
                Text('Entek sargyt ýok', textAlign: TextAlign.center, style: TextStyle(color: TaxiColors.grey)),
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
                    child: Padding(
                      padding: const EdgeInsets.all(14),
                      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                        Row(children: [
                          Text('#${o.id} · ${dateTime(o.createdAt)}', style: const TextStyle(color: TaxiColors.grey)),
                          const Spacer(),
                          StatusChip(o.status, delivery: o.isDelivery),
                        ]),
                        const SizedBox(height: 10),
                        RouteText(from: o.pickupAddress, to: o.dropoffAddress),
                        const SizedBox(height: 10),
                        Row(children: [
                          Text('${paymentText(o.paymentMethod)} · ${km(o.distanceKm)}', style: const TextStyle(color: TaxiColors.grey)),
                          if (o.commission > 0) Text(' · kom. ${money(o.commission, cur)}', style: const TextStyle(color: TaxiColors.grey)),
                          const Spacer(),
                          Text(money(o.price, cur), style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 16)),
                        ]),
                      ]),
                    ),
                  );
                },
              ),
      ),
    );
  }
}
