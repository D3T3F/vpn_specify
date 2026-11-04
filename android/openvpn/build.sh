#!/bin/bash
# Builds OpenVPN for Android, unmodified, from the official sources. The result
# is android/app/src/main/jniLibs/<abi>/libovpnexec.so, which is kept in the
# repository so that the app builds on any system without this script.
#
# Runs on Linux or WSL. Needs: gcc make perl python3 cmake pkg-config curl unzip
#   sudo apt install cmake pkg-config
#
#   android/openvpn/build.sh [abi...]      (default: arm64-v8a x86_64)
#
# GPL: this script, with the pinned sources below, is how the binaries in the APK
# are made. Publish it, unchanged, along with each release.
set -euo pipefail

OPENVPN=2.7.8
OPENVPN_SHA=c070d1d2440b5a6fca6c2c68645c98cd492116ac36ef4f0946177115532c8e36
OPENSSL=3.5.9
OPENSSL_SHA=603f5602e2eef00d77fbd429d34dcd5822bb301757a1bc9cdb24c670f1eb859a
LZ4=1.10.0
LZ4_SHA=537512904744b35e232912055ccf8ec66d768639ff3abe5788d90d792ec5f48b
LZO=2.10
LZO_SHA=c0f892943208266f9b6543b3ae308fab6284c5c90e627931446fb49b4221a072
NDK=r27b # the same as ndkVersion in android/app/build.gradle.kts
NDK_SHA=33e16af1a6bbabe12cad54b2117085c07eab7e4fa67cdd831805f0e94fd826c1
API=26

here=$(cd "$(dirname "$0")" && pwd)
out=$here/../app/src/main/jniLibs
work=${VPN_SPECIFY_BUILD:-$HOME/.cache/vpn_specify-openvpn}
mkdir -p "$work/dl"
jobs=$(nproc)

# fetch url sha256 var: sets var to the path of the verified download. It has no
# output to capture on purpose: inside $(...) a failed check would not stop the script.
fetch() {
	local file=$work/dl/$(basename "$1")
	if [ ! -f "$file" ]; then
		curl -sSfL -o "$file.part" "$1"
		mv "$file.part" "$file"
	fi
	if ! echo "$2  $file" | sha256sum -c --quiet - >&2; then
		echo "wrong SHA-256: $file" >&2
		exit 1
	fi
	printf -v "$3" %s "$file"
}

fetch "https://dl.google.com/android/repository/android-ndk-$NDK-linux.zip" "$NDK_SHA" ndk_zip
fetch "https://swupdate.openvpn.org/community/releases/openvpn-$OPENVPN.tar.gz" "$OPENVPN_SHA" openvpn_tgz
fetch "https://github.com/openssl/openssl/releases/download/openssl-$OPENSSL/openssl-$OPENSSL.tar.gz" "$OPENSSL_SHA" openssl_tgz
fetch "https://github.com/lz4/lz4/releases/download/v$LZ4/lz4-$LZ4.tar.gz" "$LZ4_SHA" lz4_tgz
fetch "https://www.oberhumer.com/opensource/lzo/download/lzo-$LZO.tar.gz" "$LZO_SHA" lzo_tgz

ndk=$work/android-ndk-$NDK
[ -d "$ndk" ] || unzip -q "$ndk_zip" -d "$work"
bin=$ndk/toolchains/llvm/prebuilt/linux-x86_64/bin

build_abi() {
	local abi=$1 ossl triple
	case $abi in
	arm64-v8a) ossl=android-arm64 triple=aarch64-linux-android ;;
	x86_64) ossl=android-x86_64 triple=x86_64-linux-android ;;
	*) echo "unknown abi: $abi" >&2; exit 1 ;;
	esac
	local src=$work/src-$abi prefix=$work/prefix-$abi
	rm -rf "$src" "$prefix"
	mkdir -p "$src" "$prefix/lib/pkgconfig" "$prefix/include"
	local cc=$bin/$triple$API-clang
	local flags="-fPIC -O2"

	echo "== $abi: OpenSSL $OPENSSL"
	tar xzf "$openssl_tgz" -C "$src"
	(
		cd "$src/openssl-$OPENSSL"
		# no-module links the legacy provider in: old configurations use BF-CBC
		PATH=$bin:$PATH ANDROID_NDK_ROOT=$ndk ./Configure "$ossl" "-D__ANDROID_API__=$API" \
			no-shared no-tests no-docs no-apps no-module no-ui-console no-engine \
			--prefix="$prefix" --libdir=lib >/dev/null
		PATH=$bin:$PATH make -s -j"$jobs" build_libs >/dev/null
		PATH=$bin:$PATH make -s install_dev >/dev/null
	)

	echo "== $abi: LZ4 $LZ4"
	tar xzf "$lz4_tgz" -C "$src"
	make -s -C "$src/lz4-$LZ4/lib" -j"$jobs" liblz4.a CC="$cc" AR="$bin/llvm-ar" CFLAGS="$flags" >/dev/null
	cp "$src/lz4-$LZ4/lib/liblz4.a" "$prefix/lib/"
	cp "$src/lz4-$LZ4/lib/lz4.h" "$src/lz4-$LZ4/lib/lz4hc.h" "$src/lz4-$LZ4/lib/lz4frame.h" "$prefix/include/"
	cat >"$prefix/lib/pkgconfig/liblz4.pc" <<-EOF
		Name: lz4
		Description: LZ4
		Version: $LZ4
		Libs: -L$prefix/lib -llz4
		Cflags: -I$prefix/include
	EOF

	echo "== $abi: LZO $LZO"
	tar xzf "$lzo_tgz" -C "$src"
	(
		cd "$src/lzo-$LZO"
		./configure -q --host="$triple" --prefix="$prefix" --disable-shared --enable-static \
			CC="$cc" AR="$bin/llvm-ar" RANLIB="$bin/llvm-ranlib" CFLAGS="$flags" >/dev/null
		make -s -j"$jobs" install >/dev/null
	)

	echo "== $abi: OpenVPN $OPENVPN"
	tar xzf "$openvpn_tgz" -C "$src"
	PKG_CONFIG_LIBDIR=$prefix/lib/pkgconfig PKG_CONFIG_PATH= cmake -S "$src/openvpn-$OPENVPN" -B "$src/build" \
		-DCMAKE_TOOLCHAIN_FILE="$ndk/build/cmake/android.toolchain.cmake" \
		-DANDROID_ABI="$abi" -DANDROID_PLATFORM="android-$API" -DANDROID_SUPPORT_FLEXIBLE_PAGE_SIZES=ON \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_EXE_LINKER_FLAGS="-Wl,-z,max-page-size=16384 -Wl,--build-id=none" \
		-DOPENSSL_ROOT_DIR="$prefix" -DOPENSSL_USE_STATIC_LIBS=ON \
		-DCMAKE_FIND_ROOT_PATH="$prefix" \
		-DUNSUPPORTED_BUILDS=ON -DENABLE_PKCS11=OFF -DUSE_WERROR=OFF -DBUILD_TESTING=OFF -DENABLE_DNS_UPDOWN_BY_DEFAULT=OFF \
		>"$src/cmake.log" 2>&1 || { tail -30 "$src/cmake.log"; exit 1; }
	cmake --build "$src/build" --target openvpn -j"$jobs" >"$src/make.log" 2>&1 || { tail -30 "$src/make.log"; exit 1; }

	# Named lib*.so because Android only makes files with that name executable,
	# when it installs them to the app's native library folder.
	mkdir -p "$out/$abi"
	"$bin/llvm-strip" -o "$out/$abi/libovpnexec.so" "$src/build/openvpn"
	echo "== $abi: $(du -h "$out/$abi/libovpnexec.so" | cut -f1) $(sha256sum "$out/$abi/libovpnexec.so" | cut -c1-16)"

	# The licenses of what went into the binary, shown in the app
	local lic=$here/licenses
	mkdir -p "$lic"
	cp "$src/openvpn-$OPENVPN/COPYING" "$lic/openvpn.txt"
	cp "$src/openssl-$OPENSSL/LICENSE.txt" "$lic/openssl.txt"
	cp "$src/lz4-$LZ4/lib/LICENSE" "$lic/lz4.txt"
	cp "$src/lzo-$LZO/COPYING" "$lic/lzo.txt"
}

if [ $# -eq 0 ]; then set -- arm64-v8a x86_64; fi
for abi in "$@"; do build_abi "$abi"; done
echo done
