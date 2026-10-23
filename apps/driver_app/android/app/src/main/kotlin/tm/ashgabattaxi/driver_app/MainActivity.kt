package tm.ashgabattaxi.driver_app

import android.app.NotificationChannel
import android.app.NotificationManager
import android.os.Build
import android.os.Bundle
import io.flutter.embedding.android.FlutterActivity

class MainActivity : FlutterActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        createNotificationChannels()
    }

    // Channel ids must match backend/internal/services/push.go: "orders" (urgent) and "ride".
    private fun createNotificationChannels() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val nm = getSystemService(NotificationManager::class.java) ?: return
        nm.createNotificationChannel(
            NotificationChannel("orders", "Sargytlar", NotificationManager.IMPORTANCE_HIGH).apply {
                description = "Täze sargytlar we sürüjiniň gelendigi"
                enableVibration(true)
            }
        )
        nm.createNotificationChannel(
            NotificationChannel("ride", "Syýahat", NotificationManager.IMPORTANCE_DEFAULT).apply {
                description = "Syýahat tamamlandy, goldaw gullugynyň jogaby"
            }
        )
    }
}
