import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

import 'driver_controller.dart';
import 'screens/driver_root.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  final session = Session(role: 'driver')..init();
  runApp(DriverApp(session: session));
}

class DriverApp extends StatelessWidget {
  final Session session;
  const DriverApp({super.key, required this.session});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Aşgabat Taxi Sürüji',
      debugShowCheckedModeBanner: false,
      theme: taxiTheme(),
      home: SessionGate(
        session: session,
        login: PhoneLoginScreen(
          session: session,
          title: 'Aşgabat Taxi Sürüji',
          subtitle: 'Sürüji hökmünde işläň we gazanyň',
          icon: Icons.drive_eta,
          allowPassword: true,
        ),
        home: () => DriverRoot(session: session, ctrl: DriverController(session)),
      ),
    );
  }
}
