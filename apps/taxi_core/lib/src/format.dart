import 'models.dart';

/// "12.5 TMT" — prices are rounded to 0.5 manat by the server.
String money(double v, [String currency = 'TMT']) {
  final s = v == v.roundToDouble() ? v.toStringAsFixed(0) : v.toStringAsFixed(1);
  return '$s $currency';
}

String km(double v) => v < 1 ? '${(v * 1000).round()} m' : '${v.toStringAsFixed(1)} km';

String two(int n) => n.toString().padLeft(2, '0');

String dateTime(DateTime? t) {
  if (t == null) return '—';
  return '${two(t.day)}.${two(t.month)}.${t.year} ${two(t.hour)}:${two(t.minute)}';
}

/// Turkmen labels for order statuses.
String orderStatusText(String s, {bool delivery = false}) => switch (s) {
      OrderStatus.searching => 'Sürüji gözlenýär',
      OrderStatus.accepted => 'Sürüji ýolda',
      OrderStatus.arrived => 'Sürüji garaşýar',
      OrderStatus.inProgress => delivery ? 'Eltilýär' : 'Ýolda',
      OrderStatus.completed => 'Tamamlandy',
      OrderStatus.cancelled => 'Ýatyryldy',
      _ => s,
    };

String paymentText(String m) => m == 'card' ? 'Kart' : 'Nagt';

/// Normalises user input to +993XXXXXXXX (same rules as backend utils.NormalizePhone).
String normalizePhone(String raw) {
  var d = raw.replaceAll(RegExp(r'\D'), '');
  if (d.length == 8) {
    d = '993$d';
  } else if (d.length == 9 && d.startsWith('8')) {
    d = '993${d.substring(1)}';
  }
  return d.isEmpty ? '' : '+$d';
}

bool validPhone(String p) => RegExp(r'^\+993\d{8}$').hasMatch(p);

/// "+993 65 12-34-56"
String prettyPhone(String p) {
  final n = normalizePhone(p);
  if (!validPhone(n)) return p;
  return '+993 ${n.substring(4, 6)} ${n.substring(6, 8)}-${n.substring(8, 10)}-${n.substring(10)}';
}
