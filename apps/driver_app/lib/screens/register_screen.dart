import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

/// Car + licence form. The profile goes to admin moderation after submit.
class RegisterScreen extends StatefulWidget {
  final Session session;
  final DriverProfile? initial;
  final void Function(DriverProfile) onDone;
  final VoidCallback? onCancel;

  const RegisterScreen({super.key, required this.session, this.initial, required this.onDone, this.onCancel});

  @override
  State<RegisterScreen> createState() => _RegisterScreenState();
}

class _RegisterScreenState extends State<RegisterScreen> {
  final form = GlobalKey<FormState>();
  late final name = TextEditingController(text: widget.session.user?.name ?? '');
  late final brand = TextEditingController(text: widget.initial?.carBrand ?? '');
  late final model = TextEditingController(text: widget.initial?.carModel ?? '');
  late final color = TextEditingController(text: widget.initial?.carColor ?? '');
  late final plate = TextEditingController(text: widget.initial?.carPlate ?? '');
  late final year = TextEditingController(text: (widget.initial?.carYear ?? 0) > 0 ? '${widget.initial!.carYear}' : '');
  late final license = TextEditingController(text: widget.initial?.licenseNumber ?? '');
  List<Tariff> tariffs = [];
  late Set<String> codes = {...(widget.initial?.tariffList ?? const ['economy'])};
  bool busy = false;

  @override
  void initState() {
    super.initState();
    widget.session.api.tariffs().then((t) {
      if (mounted) setState(() => tariffs = t);
    }).catchError((_) {});
  }

  @override
  void dispose() {
    for (final c in [name, brand, model, color, plate, year, license]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _submit() async {
    if (!form.currentState!.validate()) return;
    if (codes.isEmpty) {
      showMsg(context, 'Iň bolmanda bir tarif saýlaň', error: true);
      return;
    }
    setState(() => busy = true);
    final d = await guard(
      context,
      () => widget.session.api.driverRegister(
        name: name.text.trim(),
        carBrand: brand.text.trim(),
        carModel: model.text.trim(),
        carColor: color.text.trim(),
        carPlate: plate.text.trim(),
        carYear: int.tryParse(year.text.trim()) ?? 0,
        licenseNumber: license.text.trim(),
        tariffCodes: codes.toList(),
      ),
    );
    if (!mounted) return;
    setState(() => busy = false);
    if (d != null) {
      widget.session.refreshUser().catchError((_) {});
      widget.onDone(d);
    }
  }

  String? _req(String? v) => (v ?? '').trim().isEmpty ? 'Hökmany' : null;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.initial == null ? 'Sürüji bolmak' : 'Maglumatlary üýtget'),
        leading: widget.onCancel == null ? null : IconButton(icon: const Icon(Icons.close), onPressed: widget.onCancel),
        actions: [
          if (widget.initial == null)
            IconButton(
              tooltip: 'Çykmak',
              icon: const Icon(Icons.logout),
              onPressed: () => widget.session.logout(),
            ),
        ],
      ),
      body: Form(
        key: form,
        child: ListView(padding: const EdgeInsets.all(20), children: [
          const Text('Awtoulagyňyz barada maglumat beriň. Operator barlandan soň sargyt alyp başlap bilersiňiz.',
              style: TextStyle(color: TaxiColors.grey)),
          const SizedBox(height: 20),
          TextFormField(
            controller: name,
            textCapitalization: TextCapitalization.words,
            decoration: const InputDecoration(labelText: 'Adyňyz we familiýaňyz'),
            validator: _req,
          ),
          const SizedBox(height: 12),
          Row(children: [
            Expanded(child: TextFormField(controller: brand, decoration: const InputDecoration(labelText: 'Markasy'), validator: _req)),
            const SizedBox(width: 12),
            Expanded(child: TextFormField(controller: model, decoration: const InputDecoration(labelText: 'Modeli'))),
          ]),
          const SizedBox(height: 12),
          Row(children: [
            Expanded(child: TextFormField(controller: color, decoration: const InputDecoration(labelText: 'Reňki'))),
            const SizedBox(width: 12),
            Expanded(
              child: TextFormField(
                controller: year,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(labelText: 'Ýyly'),
                validator: (v) {
                  final y = int.tryParse((v ?? '').trim());
                  if ((v ?? '').trim().isEmpty) return null;
                  if (y == null || y < 1980 || y > DateTime.now().year + 1) return 'Nädogry ýyl';
                  return null;
                },
              ),
            ),
          ]),
          const SizedBox(height: 12),
          TextFormField(
            controller: plate,
            textCapitalization: TextCapitalization.characters,
            decoration: const InputDecoration(labelText: 'Döwlet belgisi', hintText: 'AG 1234 AG'),
            validator: _req,
          ),
          const SizedBox(height: 12),
          TextFormField(
            controller: license,
            textCapitalization: TextCapitalization.characters,
            decoration: const InputDecoration(labelText: 'Sürüjilik şahadatnamasynyň belgisi'),
            validator: _req,
          ),
          const SizedBox(height: 20),
          Text('Haýsy tarifler boýunça işleýärsiňiz?', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          Wrap(spacing: 8, runSpacing: 8, children: [
            for (final t in tariffs.isEmpty ? [const Tariff(id: 0, code: 'economy', name: 'Ekonom')] : tariffs)
              FilterChip(
                label: Text(t.name),
                selected: codes.contains(t.code),
                onSelected: (v) => setState(() => v ? codes.add(t.code) : codes.remove(t.code)),
              ),
          ]),
          const SizedBox(height: 28),
          FilledButton(onPressed: busy ? null : _submit, child: const Text('Barlaga iber')),
        ]),
      ),
    );
  }
}
