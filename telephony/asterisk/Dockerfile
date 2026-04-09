FROM andrius/asterisk:alpine-18
COPY configs/pjsip.conf /etc/asterisk/pjsip.conf
COPY configs/extensions.conf /etc/asterisk/extensions.conf
COPY configs/rtp.conf /etc/asterisk/rtp.conf
CMD ["asterisk", "-fvvv"]
