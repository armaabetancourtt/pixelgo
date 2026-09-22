package com.armaabetancourtt.pixelgo.work

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters

class TransferWorker(
    appContext: Context,
    params: WorkerParameters
) : CoroutineWorker(appContext, params) {
    override suspend fun doWork(): Result {
        // The worker boundary exists now; signed URL transfer + checksum verification lands next.
        return Result.success()
    }
}
