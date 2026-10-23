import 'dart:async';

import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

import 'topup.dart';

/// Balance, payment plan (commission / monthly), card top-up, income and balance movements.
class EarningsScreen extends StatefulWidget {
  final Session session;
  const EarningsScreen({super.key, required this.session});

  @override
  State<EarningsScreen> createState() => _EarningsScreenState();
}

class _EarningsScreenState extends State<EarningsScreen> {
  Earnings? e;
  List<Payment> payments = [];
  StreamSubscription? _ev;

  @override
  void initState() {
    super.initState();
    _load();
    // money credited by the bank or the monthly fee taken while this screen is open
    _ev = widget.session.realtime.events.where((ev) => ev.type == 'balance.changed').listen((_) => _load());
  }

  @override
  void dispose() {
    _ev?.cancel();
    super.dispose();
  }

  Future<void> _load() async {
    final r = await guard(context, widget.session.api.earnings);
    if (!mounted || r == null) return;
    setState(() => e = r);
    if (r.cardTopUp) {
      try {
        final ps = await widget.session.api.payments();
        if (mounted) setState(() => payments = ps);
      } catch (_) {}
    }
  }

  Future<void> _topUp() async {
    final x = e!;
    double? need;
    if (x.isMonthly && !x.subscriptionActive && x.balance < x.monthlyFee) need = x.monthlyFee - x.balance;
    if (!x.isMonthly && x.balance < x.minBalance) need = x.minBalance - x.balance;
    await startTopUp(context, widget.session, suggested: need, min: x.topUpMin, max: x.topUpMax);
    _load();
  }

  Future<void> _renew() async {
    final x = e!;
    final cur = widget.session.config.currency;
    if (!await confirm(context, 'Aýlyk tölegi tölemek',
        text: 'Balansdan ${money(x.monthlyFee, cur)} alnar we iş möhleti 30 gün uzalar.', ok: 'Töle', cancel: 'Yza')) {
      return;
    }
    if (!mounted) return;
    final ok = await guard(context, () async {
      await widget.session.api.renewSubscription();
      return true;
    });
    if (ok == true && mounted) showMsg(context, 'Aýlyk töleg tölendi');
    _load();
  }

  static const _types = {
    'commission': 'Komissiýa',
    'topup': 'Balans dolduryldy (ofis)',
    'card_topup': 'Kart bilen dolduryldy',
    'subscription': 'Aýlyk töleg',
    'payout': 'Töleg berildi',
    'ride_income': 'Kart töleg (syýahat)',
    'adjustment': 'Düzediş',
  };

  @override
  Widget build(BuildContext context) {
    final cur = widget.session.config.currency;
    final x = e;
    return Scaffold(
      appBar: AppBar(title: const Text('Balans we gazanç')),
      body: x == null
          ? const Center(child: CircularProgressIndicator())
          : RefreshIndicator(
              onRefresh: _load,
              child: ListView(padding: const EdgeInsets.all(16), children: [
                _balanceCard(x, cur),
                const SizedBox(height: 12),
                _planCard(x, cur),
                const SizedBox(height: 12),
                Row(children: [
                  _period('Şu gün', x.today, cur),
                  const SizedBox(width: 8),
                  _period('Hepde', x.week, cur),
                  const SizedBox(width: 8),
                  _period('Aý', x.month, cur),
                ]),
                if (payments.isNotEmpty) ...[
                  const SizedBox(height: 20),
                  Text('Kart tölegleri', style: Theme.of(context).textTheme.titleMedium),
                  for (final p in payments.take(10))
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      leading: Icon(
                        p.isPaid ? Icons.check_circle : (p.isPending ? Icons.hourglass_top : Icons.cancel),
                        color: p.isPaid ? TaxiColors.green : (p.isPending ? TaxiColors.grey : TaxiColors.red),
                      ),
                      title: Text(money(p.amount, cur)),
                      subtitle: Text('${dateTime(p.createdAt)} · ${p.isPaid ? 'tölendi' : (p.isPending ? 'garaşylýar' : 'geçmedi')}'),
                    ),
                ],
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

  Widget _balanceCard(Earnings x, String cur) {
    final low = x.isMonthly ? (!x.subscriptionActive && x.balance < x.monthlyFee) : x.balance < x.minBalance;
    return Card(
      color: TaxiColors.dark,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('Balans', style: TextStyle(color: Colors.white70)),
          Text(money(x.balance, cur),
              style: TextStyle(color: x.balance < 0 ? const Color(0xFFFF8A80) : TaxiColors.yellow, fontSize: 34, fontWeight: FontWeight.w900)),
          const SizedBox(height: 4),
          Text('★ ${x.rating.toStringAsFixed(1)} · ${x.totalRides} syýahat', style: const TextStyle(color: Colors.white70)),
          if (low) ...[
            const SizedBox(height: 8),
            const Text('Sargyt almak üçin balansy dolduryň.', style: TextStyle(color: Color(0xFFFF8A80), fontWeight: FontWeight.w600)),
          ],
          const SizedBox(height: 14),
          if (x.cardTopUp)
            FilledButton.icon(
              key: const Key('topupBtn'),
              onPressed: _topUp,
              icon: const Icon(Icons.credit_card),
              label: const Text('Kart bilen doldur'),
            )
          else
            const Text('Balansy ofisde ýa-da operator arkaly dolduryp bolýar.', style: TextStyle(color: Colors.white70)),
        ]),
      ),
    );
  }

  Widget _planCard(Earnings x, String cur) {
    final String title, text;
    if (x.isMonthly) {
      title = 'Aýlyk töleg: ${money(x.monthlyFee, cur)} / 30 gün';
      text = x.subscriptionActive
          ? '${dateTime(x.subscriptionUntil)} çenli tölenen. Syýahatlardan komissiýa alynmaýar.'
          : 'Möhleti gutardy. Balansda ${money(x.monthlyFee, cur)} bolsa, işe başlanyňyzda özi tölener.';
    } else {
      title = 'Göterim: ${(x.commissionRate * 100).round()}% her syýahatdan';
      text = 'Nagt syýahatda komissiýa balansdan aýrylýar. Balans ${money(x.minBalance, cur)}-den az bolsa sargyt alyp bolmaýar.';
    }
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            Icon(x.isMonthly ? Icons.calendar_month : Icons.percent, color: TaxiColors.dark),
            const SizedBox(width: 8),
            Expanded(child: Text(title, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 16))),
          ]),
          const SizedBox(height: 6),
          Text(text, style: const TextStyle(color: TaxiColors.grey)),
          if (x.isMonthly) ...[
            const SizedBox(height: 10),
            OutlinedButton(
              onPressed: _renew,
              child: Text(x.subscriptionActive ? 'Indiki aýy öňünden töle' : 'Aýlyk tölegi balansdan töle'),
            ),
          ],
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
