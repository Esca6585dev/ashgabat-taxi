import 'package:flutter/material.dart';
import 'package:taxi_core/taxi_core.dart';

import 'ride_controller.dart';
import 'screens/home_screen.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  final session = Session(role: 'client')..init();
  PushService.start(session);
  runApp(ClientApp(session: session));
}

class ClientApp extends StatelessWidget {
  final Session session;
  const ClientApp({super.key, required this.session});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Aşgabat Taxi',
      debugShowCheckedModeBanner: false,
      theme: taxiTheme(),
      home: SessionGate(
        session: session,
        login: PhoneLoginScreen(session: session, title: 'Aşgabat Taxi', subtitle: 'Şäheriň islendik ýerine çalt we arzan'),
        home: () => HomeScreen(session: session, ride: RideController(session)),
      ),
    );
  }
}
