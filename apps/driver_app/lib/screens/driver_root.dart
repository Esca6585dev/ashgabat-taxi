import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

import '../driver_controller.dart';
import 'driver_home.dart';
import 'register_screen.dart';

/// Routes the driver by profile state: register -> waiting for approval -> work screen.
class DriverRoot extends StatefulWidget {
  final Session session;
  final DriverController ctrl;
  const DriverRoot({super.key, required this.session, required this.ctrl});

  @override
  State<DriverRoot> createState() => _DriverRootState();
}

class _DriverRootState extends State<DriverRoot> {
  bool editing = false;

  @override
  void initState() {
    super.initState();
    widget.ctrl.refresh();
  }

  @override
  void dispose() {
    widget.ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.ctrl,
      builder: (context, _) {
        final c = widget.ctrl;
        if (!c.loaded) return const Scaffold(body: Center(child: CircularProgressIndicator()));
        if (c.driver == null && c.loadError != null) {
          return _Message(
            icon: Icons.wifi_off_rounded,
            title: c.loadError!,
            action: 'Täzeden synanyş',
            onAction: c.refresh,
            session: widget.session,
          );
        }
        final d = c.driver;
        if (d == null || editing) {
          return RegisterScreen(
            session: widget.session,
            initial: d,
            onDone: (p) {
              setState(() => editing = false);
              c.registered(p);
            },
            onCancel: d == null ? null : () => setState(() => editing = false),
          );
        }
        switch (d.status) {
          case DriverStatus.pending:
            return _Message(
              icon: Icons.hourglass_top_rounded,
              title: 'Profiliňiz barlanýar',
              text: 'Operator maglumatlaryňyzy barlap tassyklar. Adatça bu 1 iş gününden az wagt alýar.\n\n'
                  '${d.carTitle} · ${d.carPlate}',
              action: 'Ýagdaýy barla',
              onAction: c.refresh,
              secondary: 'Maglumatlary üýtget',
              onSecondary: () => setState(() => editing = true),
              session: widget.session,
            );
          case DriverStatus.rejected:
            return _Message(
              icon: Icons.error_outline,
              title: 'Profil ret edildi',
              text: d.rejectReason.isEmpty ? 'Maglumatlary düzedip täzeden iberiň.' : 'Sebäbi: ${d.rejectReason}',
              action: 'Düzedip täzeden iber',
              onAction: () => setState(() => editing = true),
              session: widget.session,
            );
          case DriverStatus.blocked:
            return _Message(
              icon: Icons.block,
              title: 'Sürüji hasabyňyz bloklandy',
              text: d.rejectReason.isEmpty ? 'Jikme-jiklikler üçin goldaw gullugyna jaň ediň.' : 'Sebäbi: ${d.rejectReason}',
              action: 'Goldaw gullugyna jaň et',
              onAction: () => callPhone(widget.session.config.supportPhone),
              session: widget.session,
            );
          default:
            return DriverHome(session: widget.session, ctrl: c);
        }
      },
    );
  }
}

class _Message extends StatelessWidget {
  final IconData icon;
  final String title;
  final String? text;
  final String action;
  final VoidCallback onAction;
  final String? secondary;
  final VoidCallback? onSecondary;
  final Session session;

  const _Message({
    required this.icon,
    required this.title,
    this.text,
    required this.action,
    required this.onAction,
    this.secondary,
    this.onSecondary,
    required this.session,
  });

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(actions: [
        IconButton(
          tooltip: 'Çykmak',
          icon: const Icon(Icons.logout),
          onPressed: () async {
            if (await confirm(context, 'Hasapdan çykmalymy?')) session.logout();
          },
        ),
      ]),
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(32),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            Icon(icon, size: 72, color: TaxiColors.yellow),
            const SizedBox(height: 16),
            Text(title, style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w800), textAlign: TextAlign.center),
            if (text != null) ...[
              const SizedBox(height: 12),
              Text(text!, textAlign: TextAlign.center, style: const TextStyle(color: TaxiColors.grey, fontSize: 16)),
            ],
            const SizedBox(height: 28),
            FilledButton(onPressed: onAction, child: Text(action)),
            if (secondary != null) TextButton(onPressed: onSecondary, child: Text(secondary!)),
          ]),
        ),
      ),
    );
  }
}
