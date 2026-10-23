import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

import '../driver_controller.dart';
import 'register_screen.dart';

class DriverProfileScreen extends StatelessWidget {
  final Session session;
  final DriverController ctrl;
  const DriverProfileScreen({super.key, required this.session, required this.ctrl});

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: Listenable.merge([ctrl, session]),
      builder: (context, _) {
        final u = session.user;
        final d = ctrl.driver;
        Widget row(String k, String v) => ListTile(
              dense: true,
              title: Text(k, style: const TextStyle(color: TaxiColors.grey)),
              trailing: Text(v.isEmpty ? '—' : v, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
            );
        return Scaffold(
          appBar: AppBar(title: const Text('Profil')),
          body: ListView(padding: const EdgeInsets.symmetric(vertical: 12), children: [
            row('Ady', u?.name ?? ''),
            row('Telefon', prettyPhone(u?.phone ?? '')),
            row('Reýting', '★ ${(u?.rating ?? 5).toStringAsFixed(1)} (${u?.ratingCount ?? 0})'),
            const Divider(),
            row('Awtoulag', d?.carTitle ?? ''),
            row('Belgisi', d?.carPlate ?? ''),
            row('Ýyly', (d?.carYear ?? 0) > 0 ? '${d!.carYear}' : ''),
            row('Şahadatnama', d?.licenseNumber ?? ''),
            row('Tarifler', d?.tariffCodes ?? ''),
            row('Komissiýa', '${((d?.commissionRate ?? 0) * 100).round()}%'),
            const SizedBox(height: 12),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                OutlinedButton.icon(
                  onPressed: ctrl.busy
                      ? null
                      : () async {
                          if (!await confirm(context, 'Maglumatlary üýtgetmek',
                              text: 'Döwlet belgisini ýa-da şahadatnamany üýtgetseňiz, profil täzeden barlaga gidýär we tassyklanýança sargyt alyp bilmersiňiz. Dowam etmelimi?')) {
                            return;
                          }
                          if (!context.mounted) return;
                          Navigator.push(
                            context,
                            MaterialPageRoute(
                              builder: (pc) => RegisterScreen(
                                session: session,
                                initial: d,
                                onDone: (p) {
                                  ctrl.registered(p);
                                  ctrl.refresh();
                                  Navigator.pop(pc);
                                },
                                onCancel: () => Navigator.pop(pc),
                              ),
                            ),
                          );
                        },
                  icon: const Icon(Icons.edit_outlined),
                  label: const Text('Awtoulag maglumatlaryny üýtget'),
                ),
                const SizedBox(height: 12),
                OutlinedButton.icon(
                  onPressed: () => _changePassword(context),
                  icon: const Icon(Icons.key_outlined),
                  label: Text(u?.hasPassword == true ? 'Paroly üýtget' : 'Parol bellemek (SMS-siz girmek üçin)'),
                ),
                const SizedBox(height: 12),
                OutlinedButton.icon(
                  style: OutlinedButton.styleFrom(foregroundColor: TaxiColors.red),
                  onPressed: ctrl.busy
                      ? null
                      : () async {
                          if (!await confirm(context, 'Hasapdan çykmalymy?')) return;
                          if (ctrl.online) await ctrl.setOnline(false).catchError((_) {});
                          await session.logout();
                          if (context.mounted) Navigator.of(context).popUntil((r) => r.isFirst);
                        },
                  icon: const Icon(Icons.logout),
                  label: const Text('Çykmak'),
                ),
              ]),
            ),
          ]),
        );
      },
    );
  }

  Future<void> _changePassword(BuildContext context) async {
    final has = session.user?.hasPassword == true;
    final oldC = TextEditingController();
    final newC = TextEditingController();
    final ok = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        title: Text(has ? 'Paroly üýtget' : 'Parol bellemek'),
        content: Column(mainAxisSize: MainAxisSize.min, children: [
          if (!has)
            const Padding(
              padding: EdgeInsets.only(bottom: 12),
              child: Text('Indiki gezek telefon belgiňiz we şu parol bilen SMS kodsyz girip bilersiňiz.'),
            ),
          if (has) TextField(controller: oldC, obscureText: true, decoration: const InputDecoration(labelText: 'Häzirki parol')),
          if (has) const SizedBox(height: 12),
          TextField(controller: newC, obscureText: true, decoration: const InputDecoration(labelText: 'Täze parol (azyndan 6 simwol)')),
        ]),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c, false), child: const Text('Ýap')),
          TextButton(onPressed: () => Navigator.pop(c, true), child: const Text('Ýatda sakla')),
        ],
      ),
    );
    final oldP = oldC.text, newP = newC.text;
    oldC.dispose();
    newC.dispose();
    if (ok != true || !context.mounted) return;
    if (newP.length < 6) {
      showMsg(context, 'Parol azyndan 6 simwol bolmaly', error: true);
      return;
    }
    final done = await guard(context, () async {
      await session.api.setPassword(newP, oldPassword: oldP);
      await session.refreshUser();
      return true;
    });
    if (done == true && context.mounted) showMsg(context, 'Parol ýatda saklandy');
  }
}
