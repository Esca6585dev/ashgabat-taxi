import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api.dart';
import '../format.dart';
import '../models.dart';
import '../session.dart';
import 'theme.dart';

void showMsg(BuildContext context, Object msg, {bool error = false}) {
  final m = ScaffoldMessenger.maybeOf(context);
  m?.hideCurrentSnackBar();
  m?.showSnackBar(SnackBar(content: Text('$msg'), backgroundColor: error ? TaxiColors.red : TaxiColors.dark));
}

/// Runs [f], shows the API error as a snackbar, returns null on failure.
Future<T?> guard<T>(BuildContext context, Future<T> Function() f) async {
  try {
    return await f();
  } catch (e) {
    if (context.mounted) showMsg(context, e is ApiException ? e.message : 'Ýalňyşlyk: $e', error: true);
    return null;
  }
}

Future<void> callPhone(String phone) async {
  if (phone.isEmpty) return;
  await launchUrl(Uri(scheme: 'tel', path: normalizePhone(phone)));
}

Future<bool> confirm(BuildContext context, String title, {String? text, String ok = 'Hawa', String cancel = 'Ýok'}) async {
  final r = await showDialog<bool>(
    context: context,
    builder: (c) => AlertDialog(
      title: Text(title),
      content: text == null ? null : Text(text),
      actions: [
        TextButton(onPressed: () => Navigator.pop(c, false), child: Text(cancel)),
        FilledButton(
          style: FilledButton.styleFrom(minimumSize: const Size(90, 44)),
          onPressed: () => Navigator.pop(c, true),
          child: Text(ok),
        ),
      ],
    ),
  );
  return r == true;
}

/// Asks for a short free-text reason (cancel reasons) with quick choices.
Future<String?> askReason(BuildContext context, String title, List<String> choices) {
  return showModalBottomSheet<String>(
    context: context,
    showDragHandle: true,
    builder: (c) => SafeArea(
      child: Column(mainAxisSize: MainAxisSize.min, children: [
        Padding(padding: const EdgeInsets.fromLTRB(20, 0, 20, 8), child: Text(title, style: Theme.of(c).textTheme.titleLarge)),
        for (final ch in choices) ListTile(title: Text(ch), onTap: () => Navigator.pop(c, ch)),
        const SizedBox(height: 8),
      ]),
    ),
  );
}

/// Bottom sheet with 1-5 stars and an optional comment. Returns (stars, comment) or null.
Future<(int, String)?> askRating(BuildContext context, String title, {bool withComment = true}) {
  return showModalBottomSheet<(int, String)>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (c) => _RatingSheet(title: title, withComment: withComment),
  );
}

class _RatingSheet extends StatefulWidget {
  final String title;
  final bool withComment;
  const _RatingSheet({required this.title, required this.withComment});

  @override
  State<_RatingSheet> createState() => _RatingSheetState();
}

class _RatingSheetState extends State<_RatingSheet> {
  int stars = 5;
  final comment = TextEditingController();

  @override
  void dispose() {
    comment.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.fromLTRB(20, 0, 20, MediaQuery.of(context).viewInsets.bottom + 20),
      child: Column(mainAxisSize: MainAxisSize.min, children: [
        Text(widget.title, style: Theme.of(context).textTheme.titleLarge, textAlign: TextAlign.center),
        const SizedBox(height: 12),
        Row(mainAxisAlignment: MainAxisAlignment.center, children: [
          for (var i = 1; i <= 5; i++)
            IconButton(
              iconSize: 42,
              onPressed: () => setState(() => stars = i),
              icon: Icon(i <= stars ? Icons.star_rounded : Icons.star_outline_rounded, color: TaxiColors.yellow),
            ),
        ]),
        if (widget.withComment) ...[
          const SizedBox(height: 8),
          TextField(controller: comment, decoration: const InputDecoration(hintText: 'Teswir (hökmany däl)'), maxLines: 2),
        ],
        const SizedBox(height: 16),
        FilledButton(onPressed: () => Navigator.pop(context, (stars, comment.text.trim())), child: const Text('Baha ber')),
      ]),
    );
  }
}

/// Phone -> SMS code -> (name for new users). Calls [Session.verify].
/// With [allowPassword] (driver app) a second tab logs in with phone + password instead.
class PhoneLoginScreen extends StatefulWidget {
  final Session session;
  final String title;
  final String subtitle;
  final IconData icon;
  final bool allowPassword;

  const PhoneLoginScreen({
    super.key,
    required this.session,
    required this.title,
    required this.subtitle,
    this.icon = Icons.local_taxi,
    this.allowPassword = false,
  });

  @override
  State<PhoneLoginScreen> createState() => _PhoneLoginScreenState();
}

class _PhoneLoginScreenState extends State<PhoneLoginScreen> {
  final phone = TextEditingController();
  final code = TextEditingController();
  final name = TextEditingController();
  final password = TextEditingController();
  bool usePassword = false;
  bool showPassword = false;
  bool codeSent = false;
  bool busy = false;
  String? devCode;

  @override
  void dispose() {
    phone.dispose();
    code.dispose();
    name.dispose();
    password.dispose();
    super.dispose();
  }

  String get _phone => normalizePhone(phone.text);

  Future<void> _send() async {
    if (!validPhone(_phone)) {
      showMsg(context, 'Telefon belgisini dogry giriziň: 6X XXXXXX', error: true);
      return;
    }
    setState(() => busy = true);
    final r = await guard(context, () async => (await widget.session.sendOtp(_phone)) ?? '');
    if (!mounted) return;
    setState(() {
      busy = false;
      if (r != null) {
        codeSent = true;
        devCode = r.isEmpty ? null : r;
      }
    });
  }

  Future<void> _verify() async {
    if (code.text.trim().length < 4) {
      showMsg(context, 'SMS kody giriziň', error: true);
      return;
    }
    setState(() => busy = true);
    await guard(context, () => widget.session.verify(_phone, code.text.trim(), name: name.text.trim()));
    if (mounted) setState(() => busy = false);
  }

  Future<void> _passwordLogin() async {
    if (!validPhone(_phone)) {
      showMsg(context, 'Telefon belgisini dogry giriziň: 6X XXXXXX', error: true);
      return;
    }
    if (password.text.isEmpty) {
      showMsg(context, 'Paroly giriziň', error: true);
      return;
    }
    setState(() => busy = true);
    await guard(context, () => widget.session.loginWithPassword(_phone, password.text));
    if (mounted) setState(() => busy = false);
  }

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final pwMode = widget.allowPassword && usePassword;
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                Container(
                  width: 84,
                  height: 84,
                  alignment: Alignment.center,
                  decoration: const BoxDecoration(color: TaxiColors.yellow, shape: BoxShape.circle),
                  child: Icon(widget.icon, size: 46, color: TaxiColors.dark),
                ),
                const SizedBox(height: 20),
                Text(widget.title, style: t.headlineMedium?.copyWith(fontWeight: FontWeight.w800), textAlign: TextAlign.center),
                const SizedBox(height: 6),
                Text(widget.subtitle, style: t.bodyLarge?.copyWith(color: TaxiColors.grey), textAlign: TextAlign.center),
                const SizedBox(height: 32),
                if (widget.allowPassword) ...[
                  SegmentedButton<bool>(
                    segments: const [
                      ButtonSegment(value: false, icon: Icon(Icons.sms_outlined), label: Text('SMS kod')),
                      ButtonSegment(value: true, icon: Icon(Icons.key_outlined), label: Text('Parol')),
                    ],
                    selected: {usePassword},
                    onSelectionChanged: busy ? null : (v) => setState(() => usePassword = v.first),
                  ),
                  const SizedBox(height: 16),
                ],
                TextField(
                  key: const Key('phone'),
                  controller: phone,
                  enabled: pwMode || !codeSent,
                  keyboardType: TextInputType.phone,
                  inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9 +\-]')), LengthLimitingTextInputFormatter(16)],
                  decoration: const InputDecoration(prefixText: '+993 ', labelText: 'Telefon belgi', hintText: '6X XXXXXX'),
                  style: const TextStyle(fontSize: 18),
                ),
                if (pwMode) ...[
                  const SizedBox(height: 12),
                  TextField(
                    key: const Key('password'),
                    controller: password,
                    obscureText: !showPassword,
                    onSubmitted: (_) => busy ? null : _passwordLogin(),
                    decoration: InputDecoration(
                      labelText: 'Parol',
                      helperText: 'Paroly operator berýär ýa-da profilde bellenýär',
                      suffixIcon: IconButton(
                        icon: Icon(showPassword ? Icons.visibility_off : Icons.visibility),
                        onPressed: () => setState(() => showPassword = !showPassword),
                      ),
                    ),
                  ),
                ],
                if (!pwMode && codeSent) ...[
                  const SizedBox(height: 12),
                  TextField(
                    key: const Key('code'),
                    controller: code,
                    autofocus: true,
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly, LengthLimitingTextInputFormatter(6)],
                    decoration: InputDecoration(labelText: 'SMS kod', helperText: devCode == null ? null : 'Synag kody: $devCode'),
                    style: const TextStyle(fontSize: 22, letterSpacing: 6),
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: name,
                    textCapitalization: TextCapitalization.words,
                    decoration: const InputDecoration(labelText: 'Adyňyz (täze ulanyjy üçin)'),
                  ),
                ],
                const SizedBox(height: 24),
                FilledButton(
                  key: const Key('loginBtn'),
                  onPressed: busy ? null : (pwMode ? _passwordLogin : (codeSent ? _verify : _send)),
                  child: busy
                      ? const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2.5))
                      : Text(pwMode || codeSent ? 'Girmek' : 'SMS kod al'),
                ),
                if (!pwMode && codeSent)
                  TextButton(
                    onPressed: busy ? null : () => setState(() => codeSent = false),
                    child: const Text('Belgini üýtgetmek / kody täzeden almak'),
                  ),
              ]),
            ),
          ),
        ),
      ),
    );
  }
}

/// Shows a spinner until [Session.init] finishes, a retry screen when offline,
/// the login screen when logged out, otherwise [home].
class SessionGate extends StatelessWidget {
  final Session session;
  final Widget login;
  final Widget Function() home;

  const SessionGate({super.key, required this.session, required this.login, required this.home});

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: session,
      builder: (context, _) {
        if (!session.ready) {
          return const Scaffold(body: Center(child: CircularProgressIndicator()));
        }
        if (session.user == null && session.startupError != null) {
          return Scaffold(
            body: Center(
              child: Padding(
                padding: const EdgeInsets.all(32),
                child: Column(mainAxisSize: MainAxisSize.min, children: [
                  const Icon(Icons.wifi_off_rounded, size: 64, color: TaxiColors.grey),
                  const SizedBox(height: 16),
                  Text(session.startupError!, textAlign: TextAlign.center),
                  const SizedBox(height: 24),
                  FilledButton(
                    onPressed: () {
                      session.ready = false;
                      session.init();
                    },
                    child: const Text('Täzeden synanyş'),
                  ),
                ]),
              ),
            ),
          );
        }
        return session.loggedIn ? home() : login;
      },
    );
  }
}

/// Support tickets list + new message (same for both apps).
class SupportScreen extends StatefulWidget {
  final Session session;
  final int? orderId;
  const SupportScreen({super.key, required this.session, this.orderId});

  @override
  State<SupportScreen> createState() => _SupportScreenState();
}

class _SupportScreenState extends State<SupportScreen> {
  final msg = TextEditingController();
  List<SupportTicket>? tickets;
  bool sending = false;

  @override
  void initState() {
    super.initState();
    if (widget.orderId != null) msg.text = 'Sargyt #${widget.orderId}: ';
    _load();
  }

  @override
  void dispose() {
    msg.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    final t = await guard(context, widget.session.api.myTickets);
    if (mounted) setState(() => tickets = t ?? []);
  }

  Future<void> _send() async {
    if (msg.text.trim().isEmpty) return;
    setState(() => sending = true);
    final t = await guard(context, () => widget.session.api.createTicket(msg.text.trim(), orderId: widget.orderId));
    if (!mounted) return;
    setState(() => sending = false);
    if (t != null) {
      msg.clear();
      showMsg(context, 'Habaryňyz iberildi. Operator jogap berer.');
      _load();
    }
  }

  @override
  Widget build(BuildContext context) {
    final phone = widget.session.config.supportPhone;
    return Scaffold(
      appBar: AppBar(title: const Text('Goldaw gullugy')),
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(padding: const EdgeInsets.all(16), children: [
          if (phone.isNotEmpty)
            Card(
              child: ListTile(
                leading: const Icon(Icons.phone_in_talk, color: TaxiColors.green),
                title: const Text('Operatora jaň et'),
                subtitle: Text(prettyPhone(phone)),
                onTap: () => callPhone(phone),
              ),
            ),
          const SizedBox(height: 16),
          TextField(controller: msg, maxLines: 4, decoration: const InputDecoration(hintText: 'Soragyňyzy ýa-da şikaýatyňyzy ýazyň...')),
          const SizedBox(height: 12),
          FilledButton(onPressed: sending ? null : _send, child: const Text('Ibermek')),
          const SizedBox(height: 24),
          Text('Meniň ýüztutmalarym', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          if (tickets == null) const Center(child: CircularProgressIndicator()),
          if (tickets != null && tickets!.isEmpty) const Text('Entek ýüztutma ýok', style: TextStyle(color: TaxiColors.grey)),
          for (final t in tickets ?? <SupportTicket>[])
            Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: Card(
                child: Padding(
                  padding: const EdgeInsets.all(14),
                  child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Row(children: [
                      Text('#${t.id}', style: const TextStyle(fontWeight: FontWeight.w700)),
                      const Spacer(),
                      Text(t.status == 'resolved' ? 'Jogap berildi' : 'Garaşylýar',
                          style: TextStyle(color: t.status == 'resolved' ? TaxiColors.green : TaxiColors.grey)),
                    ]),
                    const SizedBox(height: 6),
                    Text(t.message),
                    if (t.reply.isNotEmpty) ...[
                      const Divider(),
                      Text('Jogap: ${t.reply}', style: const TextStyle(fontWeight: FontWeight.w600)),
                    ],
                    const SizedBox(height: 4),
                    Text(dateTime(t.createdAt), style: const TextStyle(color: TaxiColors.grey, fontSize: 12)),
                  ]),
                ),
              ),
            ),
        ]),
      ),
    );
  }
}

/// Small coloured label for an order status.
class StatusChip extends StatelessWidget {
  final String status;
  final bool delivery;
  const StatusChip(this.status, {super.key, this.delivery = false});

  @override
  Widget build(BuildContext context) {
    final c = switch (status) {
      OrderStatus.completed => TaxiColors.green,
      OrderStatus.cancelled => TaxiColors.red,
      OrderStatus.searching => TaxiColors.grey,
      _ => TaxiColors.blue,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(color: c.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(8)),
      child: Text(orderStatusText(status, delivery: delivery), style: TextStyle(color: c, fontWeight: FontWeight.w600, fontSize: 12)),
    );
  }
}

/// Pickup → dropoff two-line block.
class RouteText extends StatelessWidget {
  final String from;
  final String to;
  const RouteText({super.key, required this.from, required this.to});

  @override
  Widget build(BuildContext context) {
    Widget row(Color c, String s) => Row(children: [
          Icon(Icons.circle, size: 10, color: c),
          const SizedBox(width: 10),
          Expanded(child: Text(s.isEmpty ? '—' : s, maxLines: 2, overflow: TextOverflow.ellipsis)),
        ]);
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      row(TaxiColors.green, from),
      const SizedBox(height: 8),
      row(TaxiColors.red, to),
    ]);
  }
}
