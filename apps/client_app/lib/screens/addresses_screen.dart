import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

import 'search_screen.dart';

/// Saved places ("Öý", "Iş"...) shown as quick chips on the home screen.
class AddressesScreen extends StatefulWidget {
  final Session session;
  const AddressesScreen({super.key, required this.session});

  @override
  State<AddressesScreen> createState() => _AddressesScreenState();
}

class _AddressesScreenState extends State<AddressesScreen> {
  List<SavedAddress>? items;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final r = await guard(context, widget.session.api.addresses);
    if (mounted) setState(() => items = r ?? []);
  }

  Future<void> _add() async {
    final title = await _askTitle();
    if (title == null || !mounted) return;
    final p = await Navigator.push<Place>(
      context,
      MaterialPageRoute(builder: (_) => SearchScreen(session: widget.session, title: '"$title" salgysy')),
    );
    if (p == null || !mounted) return;
    await guard(context, () => widget.session.api.addAddress(title, p));
    _load();
  }

  Future<String?> _askTitle() async {
    final ctrl = TextEditingController();
    final r = await showDialog<String>(
      context: context,
      builder: (c) => AlertDialog(
        title: const Text('Salgynyň ady'),
        content: Column(mainAxisSize: MainAxisSize.min, children: [
          TextField(controller: ctrl, autofocus: true, decoration: const InputDecoration(hintText: 'Öý, Iş, Mekdep...')),
          const SizedBox(height: 8),
          Wrap(spacing: 8, children: [
            for (final s in ['Öý', 'Iş', 'Mekdep'])
              ActionChip(label: Text(s), onPressed: () => Navigator.pop(c, s)),
          ]),
        ]),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c), child: const Text('Ýap')),
          TextButton(onPressed: () => Navigator.pop(c, ctrl.text.trim()), child: const Text('Dowam et')),
        ],
      ),
    );
    ctrl.dispose();
    return r == null || r.isEmpty ? null : r;
  }

  @override
  Widget build(BuildContext context) {
    final list = items;
    return Scaffold(
      appBar: AppBar(title: const Text('Salgylarym')),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _add,
        backgroundColor: TaxiColors.yellow,
        icon: const Icon(Icons.add),
        label: const Text('Goş'),
      ),
      body: list == null
          ? const Center(child: CircularProgressIndicator())
          : list.isEmpty
              ? const Center(child: Text('Salgy ýok. «Öý» we «Iş» salgylaryňyzy goşuň.', style: TextStyle(color: TaxiColors.grey)))
              : ListView(children: [
                  for (final a in list)
                    ListTile(
                      leading: const Icon(Icons.bookmark, color: TaxiColors.yellow),
                      title: Text(a.title),
                      subtitle: Text(a.address, maxLines: 1, overflow: TextOverflow.ellipsis),
                      trailing: IconButton(
                        icon: const Icon(Icons.delete_outline),
                        onPressed: () async {
                          if (!await confirm(context, '«${a.title}» pozulsynmy?')) return;
                          if (!context.mounted) return;
                          await guard(context, () => widget.session.api.deleteAddress(a.id));
                          _load();
                        },
                      ),
                    ),
                ]),
    );
  }
}
