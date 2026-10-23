import 'dart:async';

import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';
import 'package:url_launcher/url_launcher.dart';

/// Card top-up: amount -> bank card page (browser) -> wait until the server confirms with the bank.
/// Returns true when the money reached the balance.
Future<bool> startTopUp(BuildContext context, Session session, {double? suggested, double min = 5, double max = 5000}) async {
  final amount = await showModalBottomSheet<double>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (c) => _AmountSheet(currency: session.config.currency, suggested: suggested, min: min, max: max),
  );
  if (amount == null || !context.mounted) return false;

  final started = await guard(context, () => session.api.createTopUp(amount));
  if (started == null || !context.mounted) return false;

  final uri = Uri.parse(started.formUrl);
  final opened = await launchUrl(uri, mode: LaunchMode.externalApplication).catchError((_) => false);
  if (!opened && context.mounted) {
    showMsg(context, 'Bank sahypasyny açyp bolmady', error: true);
    return false;
  }
  if (!context.mounted) return false;
  final paid = await showDialog<bool>(
    context: context,
    barrierDismissible: false,
    builder: (c) => _WaitDialog(session: session, payment: started.payment, formUrl: uri),
  );
  return paid == true;
}

class _AmountSheet extends StatefulWidget {
  final String currency;
  final double? suggested;
  final double min;
  final double max;
  const _AmountSheet({required this.currency, this.suggested, required this.min, required this.max});

  @override
  State<_AmountSheet> createState() => _AmountSheetState();
}

class _AmountSheetState extends State<_AmountSheet> {
  late final ctrl = TextEditingController(text: widget.suggested == null ? '' : widget.suggested!.ceil().toString());
  String? error;

  @override
  void dispose() {
    ctrl.dispose();
    super.dispose();
  }

  void _submit() {
    final v = double.tryParse(ctrl.text.replaceAll(',', '.').trim());
    if (v == null || v < widget.min || v > widget.max) {
      setState(() => error = 'Mukdar ${widget.min.toStringAsFixed(0)}–${widget.max.toStringAsFixed(0)} ${widget.currency} aralygynda bolmaly');
      return;
    }
    Navigator.pop(context, v);
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.fromLTRB(20, 0, 20, MediaQuery.of(context).viewInsets.bottom + 20),
      child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Text('Balansy kart bilen doldurmak', style: Theme.of(context).textTheme.titleLarge),
        const SizedBox(height: 6),
        const Text('Kart maglumatlary bankyň howpsuz sahypasynda girizilýär.', style: TextStyle(color: TaxiColors.grey)),
        const SizedBox(height: 16),
        Wrap(spacing: 8, children: [
          for (final v in const [20, 50, 100, 200, 300, 500])
            ChoiceChip(
              label: Text('$v'),
              selected: ctrl.text == '$v',
              onSelected: (_) => setState(() {
                ctrl.text = '$v';
                error = null;
              }),
            ),
        ]),
        const SizedBox(height: 12),
        TextField(
          key: const Key('topupAmount'),
          controller: ctrl,
          keyboardType: const TextInputType.numberWithOptions(decimal: true),
          onChanged: (_) => setState(() => error = null),
          decoration: InputDecoration(labelText: 'Mukdar', suffixText: widget.currency, errorText: error),
        ),
        const SizedBox(height: 16),
        FilledButton.icon(onPressed: _submit, icon: const Icon(Icons.credit_card), label: const Text('Kart bilen töle')),
      ]),
    );
  }
}

/// Polls the server (which asks the bank) until the payment is paid or failed.
class _WaitDialog extends StatefulWidget {
  final Session session;
  final Payment payment;
  final Uri formUrl;
  const _WaitDialog({required this.session, required this.payment, required this.formUrl});

  @override
  State<_WaitDialog> createState() => _WaitDialogState();
}

class _WaitDialogState extends State<_WaitDialog> {
  Timer? _timer;
  Payment? p;
  double? balance;
  bool checking = false;

  @override
  void initState() {
    super.initState();
    p = widget.payment;
    _timer = Timer.periodic(const Duration(seconds: 3), (_) => _check());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _check() async {
    if (checking) return;
    checking = true;
    try {
      final r = await widget.session.api.topUpStatus(widget.payment.id);
      if (!mounted) return;
      setState(() {
        p = r.payment;
        balance = r.balance;
      });
      if (!r.payment.isPending) _timer?.cancel();
    } catch (_) {
      // network hiccup: keep polling
    } finally {
      checking = false;
    }
  }

  @override
  Widget build(BuildContext context) {
    final cur = widget.session.config.currency;
    final pay = p!;
    return AlertDialog(
      icon: Icon(
        pay.isPaid ? Icons.check_circle : (pay.isPending ? Icons.hourglass_top : Icons.error_outline),
        color: pay.isPaid ? TaxiColors.green : (pay.isPending ? TaxiColors.yellow : TaxiColors.red),
        size: 48,
      ),
      title: Text(pay.isPaid ? 'Balans dolduryldy' : (pay.isPending ? 'Töleg garaşylýar' : 'Töleg geçmedi')),
      content: Column(mainAxisSize: MainAxisSize.min, children: [
        Text(money(pay.amount, cur), style: const TextStyle(fontSize: 28, fontWeight: FontWeight.w900)),
        const SizedBox(height: 8),
        if (pay.isPending) ...[
          const Text('Bank sahypasynda kart maglumatlaryny giriziň, soň şu ýere dolanyň.', textAlign: TextAlign.center),
          const SizedBox(height: 12),
          const LinearProgressIndicator(),
        ],
        if (pay.isPaid && balance != null) Text('Täze balans: ${money(balance!, cur)}'),
        if (!pay.isPaid && !pay.isPending && pay.error.isNotEmpty) Text(pay.error, textAlign: TextAlign.center),
      ]),
      actions: [
        if (pay.isPending)
          TextButton(
            onPressed: () => launchUrl(widget.formUrl, mode: LaunchMode.externalApplication),
            child: const Text('Bank sahypasyny aç'),
          ),
        TextButton(
          onPressed: () => Navigator.pop(context, pay.isPaid),
          child: Text(pay.isPending ? 'Soňra' : 'Bolýar'),
        ),
      ],
    );
  }
}
