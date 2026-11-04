//go:build android

// libvpn_specify.so: the Go code of the Android app. It exports one JNI
// function, Native.call(ByteArray): ByteArray; see internal/mobile. Built by
// android/app/build.gradle.kts with -buildmode=c-shared.
package main

/*
#include <jni.h>
#include <stdlib.h>

static jsize vs_len(JNIEnv *env, jbyteArray a) { return (*env)->GetArrayLength(env, a); }

static void vs_get(JNIEnv *env, jbyteArray a, jsize n, void *out) {
	(*env)->GetByteArrayRegion(env, a, 0, n, (jbyte *)out);
}

static jbyteArray vs_new(JNIEnv *env, const void *data, jsize n) {
	jbyteArray a = (*env)->NewByteArray(env, n);
	if (a != NULL) (*env)->SetByteArrayRegion(env, a, 0, n, (const jbyte *)data);
	return a;
}
*/
import "C"

import (
	"unsafe"

	"vpn_specify/internal/mobile"
)

//export Java_io_github_d3t3f_vpnspecify_Native_call
func Java_io_github_d3t3f_vpnspecify_Native_call(env *C.JNIEnv, _ C.jclass, req C.jbyteArray) C.jbyteArray {
	n := C.vs_len(env, req)
	in := make([]byte, int(n)+1) // +1: never a zero-length slice to take [0] of
	C.vs_get(env, req, n, unsafe.Pointer(&in[0]))
	out := mobile.Call(in[:n])
	p := C.CBytes(out)
	defer C.free(p)
	return C.vs_new(env, p, C.jsize(len(out)))
}

func main() {}
