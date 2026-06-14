# TurboDrop Quick Start

## Windows portable use

1. Unzip the package.
2. Double-click `START_TURBODROP.bat`.
3. The computer browser opens:
   `http://localhost:48080/dashboard.html`
4. On your phone, open the LAN URL printed in the TurboDrop window, for example:
   `http://192.168.x.x:48080/dashboard.html`

## Phone to computer

Open the phone LAN URL, go to `局域网互传`, choose phone files, then click `上传到电脑`.
Files are saved to the computer's configured save folder.

## Computer to phone

Open `http://localhost:48080/dashboard.html` on the computer, go to `局域网互传`, and use `电脑发给手机` to add files to the shared list.
On the phone LAN page, use `电脑共享文件` to download them.

## If the phone cannot open the page

1. Make sure phone and computer are on the same Wi-Fi/LAN.
2. Run `setup-firewall.bat` as Administrator once.
3. Use the LAN URL printed by TurboDrop, not `localhost` and not `0.0.0.0`.

## Access model

- Computer local URL: full dashboard and settings.
- Phone LAN URL: transfer page only. It cannot open the computer folder picker or change computer settings.
