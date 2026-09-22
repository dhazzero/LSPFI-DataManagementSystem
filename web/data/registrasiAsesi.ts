'use server';

import { prisma } from '@/lib/prisma';
import { revalidatePath } from 'next/cache';
import bcrypt from 'bcryptjs';

/**
 * Admin-side registration action (creates Pendaftaran + AsesiProfile in one go).
 * Used from the admin dashboard to register an asesi manually.
 */
export async function registerAsesiAdmin(formData: FormData) {
    try {
        const skemaId = parseInt(formData.get('skema_id') as string);
        const currentYear = new Date().getFullYear();

        // 1. Get Skema Details for the Prefixes
        const skema = await prisma.skema.findUnique({ where: { id: skemaId } });
        if (!skema) throw new Error("Skema tidak ditemukan");

        // 2. Safely Generate the 7-Digit Running Number (Transaction)
        const result = await prisma.$transaction(async (tx) => {
            // Generate sequence number per year and skema
            const seq = await tx.registrationSequence.upsert({
                where: { year_skema_id: { year: currentYear, skema_id: skemaId } },
                update: { last_seq: { increment: 1 } },
                create: { year: currentYear, skema_id: skemaId, last_seq: 1 },
            });

            // Format the Registration Number: PPL 2605 XXXXX (2605 is the fixed LSPFI License Number)
            const runningNumberFormatted = String(seq.last_seq).padStart(5, '0');
            const nomorRegistrasi = `PPL 2605 ${runningNumberFormatted}`;

            // Generate certificate sequence per year globally (across all skemas)
            const certSeq = await tx.certificateSequence.upsert({
                where: { year_skema_id: { year: currentYear, skema_id: 0 } },
                update: { last_seq: { increment: 1 } },
                create: { year: currentYear, skema_id: 0, last_seq: 1 },
            });
            const certRunningNumber = String(certSeq.last_seq).padStart(7, '0');
            const nomorSertifikat = `${skema.kode_sektor} ${skema.kode_profesi} ${skema.jenjang} ${certRunningNumber} ${currentYear}`;

            const emailPribadi = formData.get('email') as string;
            const nik = formData.get('nik') as string;
            const namaLengkap = formData.get('nama_lengkap') as string;

            // Pastikan akun User ada untuk Asesi ini (Login)
            let userAccount = await tx.user.findUnique({ where: { email: emailPribadi } });
            if (!userAccount) {
                const hashedPassword = await bcrypt.hash(nik, 10);
                userAccount = await tx.user.create({
                    data: {
                        name: namaLengkap,
                        email: emailPribadi,
                        password: hashedPassword,
                        role: 'asesi',
                    }
                });
            }
            const activeUserId = userAccount.id;

            // Upsert AsesiProfile (personal data)
            const profileData = {
                nama_lengkap: namaLengkap,
                nik: nik,
                tempat_lahir: formData.get('tempat_lahir') as string,
                tanggal_lahir: new Date(formData.get('tanggal_lahir') as string),
                jenis_kelamin: formData.get('jenis_kelamin') as string,
                kebangsaan: formData.get('kebangsaan') as string || 'Indonesia',
                alamat_lengkap: formData.get('alamat_lengkap') as string,
                kota_kabupaten: formData.get('kota_kabupaten') as string,
                provinsi: formData.get('provinsi') as string,
                no_hp: formData.get('no_hp') as string,
                email_pribadi: formData.get('email') as string,
                pendidikan_terakhir: formData.get('pendidikan_terakhir') as string,
                status_pekerjaan: formData.get('status_pekerjaan') as string,
                nama_instansi: formData.get('nama_instansi') as string || null,
                jabatan: formData.get('jabatan') as string || null,
            };

            await tx.asesiProfile.upsert({
                where: { user_id: activeUserId },
                update: profileData,
                create: {
                    user_id: activeUserId,
                    ...profileData
                },
            });

            // Create Pendaftaran (registration record)
            const newRegistration = await tx.pendaftaran.create({
                data: {
                    nomor_registrasi: nomorRegistrasi,
                    user_id: activeUserId,
                    skema_id: skemaId,
                    tujuan_asesmen: formData.get('tujuan_asesmen') as string || 'Sertifikasi',
                    sumber_anggaran: formData.get('sumber_anggaran') as string || null,
                    instansi_pemberi_anggaran: formData.get('instansi_pemberi_anggaran') as string || null,
                    kementerian: formData.get('kementerian') as string || null,
                    nama_tuk: formData.get('nama_tuk') as string || null,
                    jenis_tuk: formData.get('jenis_tuk') as string || null,
                    tanggal_uji: formData.get('tanggal_uji') ? new Date(formData.get('tanggal_uji') as string) : null,
                    kode_jadwal: formData.get('kode_jadwal') as string || null,
                    tanggal_pleno: formData.get('tanggal_pleno') ? new Date(formData.get('tanggal_pleno') as string) : null,
                    no_blanko_sertifikat: formData.get('no_blanko_sertifikat') as string || null,
                    no_sertifikat: nomorSertifikat,
                    tahun_sertifikat: formData.get('tahun_sertifikat') as string || String(currentYear),
                    no_reg_asesor: formData.get('no_reg_asesor') as string || null,
                    nama_asesor: formData.get('nama_asesor') as string || null,
                    status_pendaftaran: 'MENUNGGU_VALIDASI_ADMIN',
                },
            });

            return { nomorRegistrasi, pendaftaranId: newRegistration.id };
        });

        revalidatePath('/admin/registrasi');
        return { success: true, message: "Asesi berhasil didaftarkan!", nomor_registrasi: result.nomorRegistrasi };

    } catch (error: any) {
        return { success: false, error: error.message };
    }
}

export async function getSkemaList() {
    try {
        const skemas = await prisma.skema.findMany({
            where: { status: 'Aktif' },
            orderBy: { nama_skema: 'asc' },
        });
        return skemas;
    } catch (error) {
        return [];
    }
}
