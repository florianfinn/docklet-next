// Minimal ustar writer for small text packages (installer downloads).
// Spec: POSIX.1-1988 ustar header, 512-byte blocks, two zero blocks at the end.

export interface TarFile {
  name: string;
  content: string;
  /** Octal permission bits, e.g. 0o644. */
  mode?: number;
}

const BLOCK = 512;
const encoder = new TextEncoder();

function writeString(buf: Uint8Array, offset: number, length: number, value: string): void {
  const bytes = encoder.encode(value);
  buf.set(bytes.subarray(0, length), offset);
}

function writeOctal(buf: Uint8Array, offset: number, length: number, value: number): void {
  writeString(buf, offset, length, value.toString(8).padStart(length - 1, "0") + "\0");
}

function header(name: string, size: number, mode: number, mtime: number): Uint8Array {
  if (encoder.encode(name).length > 100) throw new Error(`tar: name too long: ${name}`);
  const h = new Uint8Array(BLOCK);
  writeString(h, 0, 100, name);
  writeOctal(h, 100, 8, mode);
  writeOctal(h, 108, 8, 0);
  writeOctal(h, 116, 8, 0);
  writeOctal(h, 124, 12, size);
  writeOctal(h, 136, 12, mtime);
  h.fill(0x20, 148, 156); // checksum field counts as spaces while summing
  h[156] = 0x30; // regular file
  writeString(h, 257, 6, "ustar\0");
  writeString(h, 263, 2, "00");
  let sum = 0;
  for (const b of h) sum += b;
  writeString(h, 148, 8, sum.toString(8).padStart(6, "0") + "\0 ");
  return h;
}

export function createTar(files: TarFile[], mtime = Math.floor(Date.now() / 1000)): Uint8Array {
  const parts: Uint8Array[] = [];
  for (const f of files) {
    const data = encoder.encode(f.content);
    parts.push(header(f.name, data.length, f.mode ?? 0o644, mtime), data);
    const pad = (BLOCK - (data.length % BLOCK)) % BLOCK;
    if (pad) parts.push(new Uint8Array(pad));
  }
  parts.push(new Uint8Array(BLOCK * 2));
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let o = 0;
  for (const p of parts) {
    out.set(p, o);
    o += p.length;
  }
  return out;
}

/** Triggers a browser download of bytes or text through a temporary object URL. */
export function downloadBlob(name: string, data: Uint8Array | string, type = "application/octet-stream"): void {
  const url = URL.createObjectURL(new Blob([data as BlobPart], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
