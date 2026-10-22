import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

class ProfileScreen extends StatefulWidget {
  final Session session;
  const ProfileScreen({super.key, required this.session});

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  late final name = TextEditingController(text: widget.session.user?.name ?? '');
  bool saving = false;

  @override
  void dispose() {
    name.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    setState(() => saving = true);
    final ok = await guard(context, () async {
      await widget.session.updateName(name.text.trim());
      return true;
    });
    if (!mounted) return;
    setState(() => saving = false);
    if (ok != null) showMsg(context, 'Ýatda saklandy');
  }

  @override
  Widget build(BuildContext context) {
    final u = widget.session.user;
    return Scaffold(
      appBar: AppBar(title: const Text('Profil')),
      body: ListView(padding: const EdgeInsets.all(20), children: [
        const Center(
          child: CircleAvatar(radius: 42, backgroundColor: TaxiColors.yellow, child: Icon(Icons.person, size: 46, color: TaxiColors.dark)),
        ),
        const SizedBox(height: 12),
        Text(prettyPhone(u?.phone ?? ''), textAlign: TextAlign.center, style: Theme.of(context).textTheme.titleMedium),
        Text('★ ${(u?.rating ?? 5).toStringAsFixed(1)} · ${u?.ratingCount ?? 0} baha', textAlign: TextAlign.center, style: const TextStyle(color: TaxiColors.grey)),
        const SizedBox(height: 24),
        TextField(controller: name, textCapitalization: TextCapitalization.words, decoration: const InputDecoration(labelText: 'Adyňyz')),
        const SizedBox(height: 12),
        FilledButton(onPressed: saving ? null : _save, child: const Text('Ýatda sakla')),
        const SizedBox(height: 32),
        OutlinedButton.icon(
          style: OutlinedButton.styleFrom(foregroundColor: TaxiColors.red),
          onPressed: () async {
            if (!await confirm(context, 'Hasapdan çykmalymy?')) return;
            await widget.session.logout();
            if (context.mounted) Navigator.of(context).popUntil((r) => r.isFirst);
          },
          icon: const Icon(Icons.logout),
          label: const Text('Çykmak'),
        ),
      ]),
    );
  }
}
