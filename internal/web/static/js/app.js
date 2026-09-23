// Küçük gelişmeler; sayfa JavaScript olmadan da tam çalışır.
(function () {
  'use strict';

  // Dönem seçici gibi alanlar seçim değişince formu gönderir.
  document.querySelectorAll('select[data-auto-submit]').forEach(function (el) {
    el.addEventListener('change', function () {
      if (el.form) el.form.submit();
    });
  });

  // Geri alınamaz işlemler için onay.
  document.querySelectorAll('form[data-confirm]').forEach(function (form) {
    form.addEventListener('submit', function (e) {
      if (!window.confirm(form.getAttribute('data-confirm'))) e.preventDefault();
    });
  });

  // "Başka burs alıyorum" işaretlenmediğinde ilgili alanları pasifleştir.
  var burs = document.querySelector('input[name="other_scholarship"]');
  if (burs) {
    var alanlar = ['other_scholarship_name', 'other_scholarship_amount']
      .map(function (n) { return document.querySelector('[name="' + n + '"]'); })
      .filter(Boolean);
    var uygula = function () {
      alanlar.forEach(function (el) {
        el.disabled = !burs.checked;
        el.closest('.field').style.opacity = burs.checked ? '1' : '.5';
      });
    };
    burs.addEventListener('change', uygula);
    uygula();
  }

  // Hane geliri ve kişi sayısından kişi başı geliri canlı göster.
  var gelir = document.getElementById('household_income');
  var kisi = document.getElementById('household_size');
  if (gelir && kisi) {
    var kutu = document.createElement('p');
    kutu.className = 'hint';
    kisi.parentNode.appendChild(kutu);
    var hesapla = function () {
      var g = parseInt(String(gelir.value).replace(/\D/g, ''), 10);
      var k = parseInt(String(kisi.value).replace(/\D/g, ''), 10);
      if (g > 0 && k > 0) {
        kutu.textContent = 'Kişi başı aylık gelir: ' +
          Math.round(g / k).toLocaleString('tr-TR') + ' ₺';
      } else {
        kutu.textContent = '';
      }
    };
    gelir.addEventListener('input', hesapla);
    kisi.addEventListener('input', hesapla);
    hesapla();
  }

})();
