import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';
import 'package:taxi_core/taxi_core.dart';

/// Address search (Nominatim via backend) + saved places + "pick on map". Pops a [Place].
class SearchScreen extends StatefulWidget {
  final Session session;
  final String title;
  final List<SavedAddress> saved;
  final LatLngPoint? near;

  const SearchScreen({super.key, required this.session, required this.title, this.saved = const [], this.near});

  @override
  State<SearchScreen> createState() => _SearchScreenState();
}

class _SearchScreenState extends State<SearchScreen> {
  final q = TextEditingController();
  Timer? _debounce;
  List<Place> results = [];
  bool loading = false;
  String _last = '';

  @override
  void dispose() {
    q.dispose();
    _debounce?.cancel();
    super.dispose();
  }

  void _changed(String v) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 450), () => _search(v.trim()));
  }

  Future<void> _search(String v) async {
    if (v == _last) return;
    _last = v;
    if (v.length < 2) {
      setState(() => results = []);
      return;
    }
    setState(() => loading = true);
    try {
      final r = await widget.session.api.search(v);
      if (mounted && v == _last) setState(() => results = r);
    } catch (_) {
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> _pickOnMap() async {
    final p = await Navigator.push<Place>(
      context,
      MaterialPageRoute(builder: (_) => MapPickScreen(session: widget.session, start: widget.near)),
    );
    if (p != null && mounted) Navigator.pop(context, p);
  }

  @override
  Widget build(BuildContext context) {
    final showSaved = q.text.trim().length < 2;
    return Scaffold(
      appBar: AppBar(
        title: TextField(
          key: const Key('searchField'),
          controller: q,
          autofocus: true,
          onChanged: _changed,
          onSubmitted: (v) => _search(v.trim()),
          decoration: InputDecoration(hintText: widget.title, filled: false, border: InputBorder.none),
          style: const TextStyle(fontSize: 18),
        ),
        actions: [
          if (q.text.isNotEmpty)
            IconButton(
              icon: const Icon(Icons.clear),
              onPressed: () {
                q.clear();
                _search('');
              },
            ),
        ],
        bottom: loading ? const PreferredSize(preferredSize: Size.fromHeight(2), child: LinearProgressIndicator(minHeight: 2)) : null,
      ),
      body: ListView(children: [
        ListTile(
          leading: const Icon(Icons.map_outlined, color: TaxiColors.blue),
          title: const Text('Kartadan saýla'),
          onTap: _pickOnMap,
        ),
        const Divider(height: 1),
        if (showSaved)
          for (final a in widget.saved)
            ListTile(
              leading: const Icon(Icons.bookmark, color: TaxiColors.yellow),
              title: Text(a.title),
              subtitle: Text(a.address, maxLines: 1, overflow: TextOverflow.ellipsis),
              onTap: () => Navigator.pop(context, a.place),
            ),
        for (final p in results)
          ListTile(
            leading: const Icon(Icons.place_outlined),
            title: Text(p.name.isNotEmpty ? p.name : p.address, maxLines: 1, overflow: TextOverflow.ellipsis),
            subtitle: p.address.isNotEmpty && p.address != p.name ? Text(p.address, maxLines: 1, overflow: TextOverflow.ellipsis) : null,
            onTap: () => Navigator.pop(context, p),
          ),
        if (!showSaved && !loading && results.isEmpty)
          const Padding(
            padding: EdgeInsets.all(24),
            child: Text('Hiç zat tapylmady. Kartadan saýlap görüň.', textAlign: TextAlign.center, style: TextStyle(color: TaxiColors.grey)),
          ),
      ]),
    );
  }
}

/// Full-screen map with a centre pin; returns the chosen point with its address.
class MapPickScreen extends StatefulWidget {
  final Session session;
  final LatLngPoint? start;
  const MapPickScreen({super.key, required this.session, this.start});

  @override
  State<MapPickScreen> createState() => _MapPickScreenState();
}

class _MapPickScreenState extends State<MapPickScreen> {
  late LatLng center;
  String address = '';
  bool resolving = false;
  Timer? _debounce;

  @override
  void initState() {
    super.initState();
    final c = widget.session.config;
    center = widget.start == null ? LatLng(c.cityLat, c.cityLng) : ll(widget.start!);
    _resolve();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    super.dispose();
  }

  void _resolve() {
    _debounce?.cancel();
    setState(() => resolving = true);
    final at = center;
    _debounce = Timer(const Duration(milliseconds: 600), () async {
      Place? p;
      try {
        p = await widget.session.api.reverse(at.latitude, at.longitude);
      } catch (_) {}
      if (!mounted || at != center) return;
      setState(() {
        resolving = false;
        address = p?.title ?? '';
      });
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Kartadan saýlaň')),
      body: Stack(children: [
        TaxiMap(
          center: center,
          zoom: 16,
          onPositionChanged: (MapCamera cam, bool gesture) {
            if (!gesture) return;
            center = cam.center;
            _resolve();
          },
        ),
        const CenterPin(),
        Align(
          alignment: Alignment.bottomCenter,
          child: Container(
            color: Colors.white,
            padding: const EdgeInsets.all(16),
            child: SafeArea(
              top: false,
              child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                Text(resolving ? 'Salgy kesgitlenýär...' : (address.isEmpty ? 'Kartadaky nokat' : address),
                    style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600), maxLines: 2),
                const SizedBox(height: 12),
                FilledButton(
                  onPressed: resolving
                      ? null
                      : () => Navigator.pop(context, Place(name: address, lat: center.latitude, lng: center.longitude)),
                  child: const Text('Şu ýer'),
                ),
              ]),
            ),
          ),
        ),
      ]),
    );
  }
}
