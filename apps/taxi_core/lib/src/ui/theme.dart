import 'package:flutter/material.dart';

/// Brand colours: taxi yellow on near-black, like the classic Ashgabat white-yellow cabs.
class TaxiColors {
  static const yellow = Color(0xFFFFC72C);
  static const dark = Color(0xFF1B1B1F);
  static const green = Color(0xFF1E9E5A);
  static const red = Color(0xFFD64545);
  static const blue = Color(0xFF2F6FED);
  static const grey = Color(0xFF8A8A93);
}

ThemeData taxiTheme({Color seed = TaxiColors.yellow}) {
  final scheme = ColorScheme.fromSeed(seedColor: seed, primary: seed, onPrimary: TaxiColors.dark, brightness: Brightness.light);
  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: const Color(0xFFF7F7F8),
    appBarTheme: const AppBarTheme(backgroundColor: Colors.white, foregroundColor: TaxiColors.dark, elevation: 0, centerTitle: false),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: seed,
        foregroundColor: TaxiColors.dark,
        minimumSize: const Size.fromHeight(54),
        textStyle: const TextStyle(fontSize: 17, fontWeight: FontWeight.w700),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: TaxiColors.dark,
        minimumSize: const Size.fromHeight(50),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: const Color(0xFFF0F0F2),
      border: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: BorderSide.none),
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
    ),
    cardTheme: CardThemeData(
      color: Colors.white,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
    ),
    snackBarTheme: const SnackBarThemeData(behavior: SnackBarBehavior.floating),
  );
}
