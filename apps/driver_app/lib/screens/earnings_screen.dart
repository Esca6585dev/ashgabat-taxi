import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

/// Today / week / month income, balance and the latest balance movements.
class EarningsScreen extends StatefulWidget {
  final Session session;
  const EarningsScreen({super.key, required this.session});

  @override
  State<EarningsScreen> createState() => _EarningsScreenState();
}

class _EarningsScreenState extends State<EarningsScreen> {
  Earnings? e;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final r = await guard(context, widget.session.api.earnings);
    if (mounted && r != null) setState(() => e = r);
  }

  static const _types = {
    'commission': 'Komissiýa',
    'topup': 'Balans dolduryldy',
    'payout': 'Töleg berildi',
    'ride_income': 'Kart töleg',
    'adjustment': 'Düzediş',
  };

  @override
  Widget build(BuildContext context) {
    final cur = widget.session.config.currency;
    final x = e;
    return Scaffold(
      appBar: AppBar(title: const Text('Gazanç')),
      body: x == null
          ? const Center(child: CircularProgressIndicator())
          : RefreshIndicator(
              onRefresh: _load,
              child: ListView(padding: const EdgeInsets.all(16), children: [
                Card(
                  color: TaxiColors.dark,
                  child: Padding(
                    padding: const EdgeInsets.all(20),
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      const Text('Balans', style: TextStyle(color: Colors.white70)),
                      Text(money(x.balance, cur),
                          style: TextStyle(color: x.balance < 0 ? const Color(0xFFFF8A80) : TaxiColors.yellow, fontSize: 34, fontWeight: FontWeight.w900)),
                      const SizedBox(height: 6),
                      Text(
                        x.balance < 0
                            ? 'Komissiýa bergiňiz bar. Balansy ofisde ýa-da operator arkaly dolduryň.'
                            : 'Komissiýa: ${(x.commissionRate * 100).round()}% · ★ ${x.rating.toStringAsFixed(1)} · ${x.totalRides} syýahat',
                        style: const TextStyle(color: Colors.white70),
                      ),
                    ]),
                  ),
                ),
                const SizedBox(height: 12),
                Row(children: [
                  _period('Şu gün', x.today, cur),
                  const SizedBox(width: 8),
                  _period('Hepde', x.week, cur),
                  const SizedBox(width: 8),
                  _period('Aý', x.month, cur),
                ]),
                const SizedBox(height: 20),
                Text('Balans hereketleri', style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 8),
                if (x.transactions.isEmpty) const Text('Entek hereket ýok', style: TextStyle(color: TaxiColors.grey)),
                for (final t in x.transactions)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    title: Text(_types[t.type] ?? t.type),
                    subtitle: Text('${dateTime(t.createdAt)}${t.orderId != null ? ' · #${t.orderId}' : ''}${t.note.isNotEmpty ? ' · ${t.note}' : ''}'),
                    trailing: Text(
                      '${t.amount > 0 ? '+' : ''}${money(t.amount, cur)}',
                      style: TextStyle(fontWeight: FontWeight.w800, color: t.amount < 0 ? TaxiColors.red : TaxiColors.green),
                    ),
                  ),
              ]),
            ),
    );
  }

  Widget _period(String label, EarningsPeriod p, String cur) => Expanded(
        child: Card(
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(label, style: const TextStyle(color: TaxiColors.grey)),
              const SizedBox(height: 4),
              FittedBox(child: Text(money(p.income, cur), style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800))),
              Text('${p.rides} syýahat', style: const TextStyle(color: TaxiColors.grey, fontSize: 12)),
            ]),
          ),
        ),
      );
}
