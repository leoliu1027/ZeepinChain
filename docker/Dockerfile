
FROM tianon/ubuntu-core:14.04
ENV ZEEPINCHAIN_PATH /var/zeepin
RUN mkdir -p $ZEEPINCHAIN_PATH
COPY zeepin $ZEEPINCHAIN_PATH
ADD config.tgz $ZEEPINCHAIN_PATH
EXPOSE 20334 20335 20336 20337 20338 20339
WORKDIR $ZEEPINCHAIN_PATH
ENTRYPOINT ["./zeepin"]

