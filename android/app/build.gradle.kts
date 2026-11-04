import java.io.ByteArrayOutputStream
import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val repoRoot: File = rootDir.parentFile

// The version comes from the same VERSION file as the desktop builds.
val appVersion: String = (findProperty("versionOverride") as String?)
    ?: repoRoot.resolve("VERSION").readText().trim()
val versionParts = appVersion.split(".").map { it.toInt() }

// Release APKs are arm64. Pass -Pabis=x86_64 to run on an emulator.
val abis: List<String> = ((findProperty("abis") as String?) ?: "arm64-v8a").split(",")

val keystoreProps = Properties().apply {
    val f = rootDir.resolve("keystore.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}

android {
    namespace = "io.github.d3t3f.vpnspecify"
    compileSdk = 36
    ndkVersion = "27.1.12297006"

    defaultConfig {
        applicationId = "io.github.d3t3f.vpnspecify"
        minSdk = 26
        targetSdk = 36
        versionName = appVersion
        versionCode = versionParts[0] * 10000 + versionParts[1] * 100 + versionParts[2]
        ndk { abiFilters += abis }
        // Only for testing the updater against a local server.
        buildConfigField(
            "String", "UPDATE_API",
            "\"${findProperty("updateApi") ?: "https://api.github.com/repos/D3T3F/vpn_specify/releases/latest"}\"",
        )
    }

    signingConfigs {
        if (keystoreProps.isNotEmpty()) {
            create("release") {
                storeFile = rootDir.resolve(keystoreProps.getProperty("storeFile"))
                storePassword = keystoreProps.getProperty("storePassword")
                keyAlias = keystoreProps.getProperty("keyAlias")
                keyPassword = keystoreProps.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        debug {
            manifestPlaceholders["cleartext"] = "true" // local test servers
        }
        release {
            manifestPlaceholders["cleartext"] = "false"
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            // Without keystore.properties the APK is signed with the debug key,
            // which is fine for testing and must not be published.
            signingConfig = signingConfigs.findByName("release") ?: signingConfigs.getByName("debug")
        }
    }

    buildFeatures { buildConfig = true }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    sourceSets["main"].jniLibs.srcDir(layout.buildDirectory.dir("goLib"))
    sourceSets["main"].assets.srcDir(layout.buildDirectory.dir("licenseAssets"))
}

kotlin {
    compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) }
}

val goArch = mapOf("arm64-v8a" to "arm64", "x86_64" to "amd64")
val clangTarget = mapOf("arm64-v8a" to "aarch64", "x86_64" to "x86_64")

// Sources that end up in the library.
val goSources = fileTree(repoRoot) {
    include("go.mod", "go.sum", "VERSION", "cmd/libvpn_specify/**", "internal/**")
    exclude("**/*_test.go")
}

fun hostTag(): String {
    val os = System.getProperty("os.name").lowercase()
    return when {
        os.contains("windows") -> "windows-x86_64"
        os.contains("mac") -> "darwin-x86_64"
        else -> "linux-x86_64"
    }
}

val goLibTasks = abis.map { abi ->
    tasks.register<Exec>("goLib_${abi.replace("-", "_")}") {
        val out = layout.buildDirectory.file("goLib/$abi/libvpn_specify.so")
        val clang = android.ndkDirectory.resolve("toolchains/llvm/prebuilt/${hostTag()}/bin/clang")
            .invariantSeparatorsPath + if (hostTag().startsWith("windows")) ".exe" else ""
        inputs.files(goSources)
        inputs.property("version", appVersion)
        outputs.file(out)
        workingDir = repoRoot
        environment("CGO_ENABLED", "1")
        environment("GOOS", "android")
        environment("GOARCH", goArch.getValue(abi))
        environment("CC", "$clang --target=${clangTarget.getValue(abi)}-linux-android26")
        commandLine(
            "go", "build", "-trimpath", "-buildmode=c-shared",
            "-ldflags=-s -w -X vpn_specify/internal/version.Version=$appVersion -extldflags=-Wl,-z,max-page-size=16384",
            "-o", out.get().asFile.path, "./cmd/libvpn_specify",
        )
    }
}

// The licenses shown in the app: vpn_specify's and those of what it links.
val licenseAssets = tasks.register("licenseAssets") {
    val outDir = layout.buildDirectory.dir("licenseAssets/licenses")
    inputs.files(repoRoot.resolve("go.sum"), repoRoot.resolve("LICENSE"))
    outputs.dir(outDir)
    doLast {
        val dest = outDir.get().asFile.apply { deleteRecursively(); mkdirs() }
        repoRoot.resolve("LICENSE").copyTo(dest.resolve("vpn_specify.txt"))
        dest.resolve("org.jetbrains.kotlin_kotlin-stdlib.txt").writeText(
            "Kotlin standard library\nCopyright JetBrains s.r.o. and Kotlin Programming Language contributors.\n\n" +
                "Licensed under the Apache License, Version 2.0: https://www.apache.org/licenses/LICENSE-2.0\n" +
                "The full text of that license is also in gvisor.dev_gvisor.txt.\n",
        )
        val listing = ByteArrayOutputStream()
        val p = ProcessBuilder(
            "go", "list", "-deps", "-f",
            "{{with .Module}}{{if not .Main}}{{.Path}}|{{.Dir}}{{end}}{{end}}", "./internal/mobile",
        ).directory(repoRoot).redirectErrorStream(false)
        p.environment()["GOOS"] = "android"
        p.environment()["GOARCH"] = "arm64"
        val proc = p.start()
        proc.inputStream.copyTo(listing)
        check(proc.waitFor() == 0) { "go list failed" }
        listing.toString().lines().filter { it.contains("|") }.distinct().forEach { line ->
            val (path, dir) = line.split("|", limit = 2)
            File(dir.trim()).listFiles()
                ?.filter { it.isFile && Regex("^(LICEN[CS]E|COPYING|NOTICE)").containsMatchIn(it.name) }
                ?.forEach { f ->
                    val suffix = if (f.name.startsWith("NOTICE")) "-NOTICE" else ""
                    f.copyTo(dest.resolve(path.replace(Regex("[/\\\\:]"), "_") + suffix + ".txt"), overwrite = true)
                }
        }
    }
}

tasks.named("preBuild") {
    dependsOn(goLibTasks)
    dependsOn(licenseAssets)
}
