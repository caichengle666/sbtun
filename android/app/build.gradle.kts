plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// 版本号来自仓库根目录的 VERSION，versionCode = 主*10000 + 次*100 + 修订。
val appVersion = rootDir.parentFile.resolve("VERSION").readText().trim().removePrefix("v")
val versionNumbers = appVersion.split(".").map { part -> part.takeWhile(Char::isDigit).toIntOrNull() ?: 0 } + listOf(0, 0, 0)
val keystoreFile: String? = System.getenv("SBTUN_KEYSTORE_FILE")?.takeIf { it.isNotBlank() }

android {
    namespace = "io.github.caichengle666.sbtun"
    compileSdk = 35

    defaultConfig {
        applicationId = "io.github.caichengle666.sbtun"
        minSdk = 24
        targetSdk = 35
        versionCode = versionNumbers[0] * 10000 + versionNumbers[1] * 100 + versionNumbers[2]
        versionName = appVersion
    }

    signingConfigs {
        if (keystoreFile != null) {
            create("release") {
                storeFile = file(keystoreFile)
                storePassword = System.getenv("SBTUN_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("SBTUN_KEY_ALIAS")
                keyPassword = System.getenv("SBTUN_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false // gomobile 生成的类靠反射调用，不做混淆
            signingConfig = signingConfigs.findByName("release")
        }
    }

    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a", "x86_64")
            isUniversalApk = true
        }
    }

    // sing-box 以 libsingbox.so 的名义打包，必须解压到 nativeLibraryDir 才能执行。
    packaging {
        jniLibs {
            useLegacyPackaging = true
            keepDebugSymbols += "**/libsingbox.so" // 官方二进制已精简，不再交给 NDK strip
        }
    }

    buildFeatures {
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    lint {
        checkReleaseBuilds = false
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.activity:activity-ktx:1.9.3")
    implementation("androidx.webkit:webkit:1.12.1")
    implementation(files("libs/sbtun.aar"))
}
