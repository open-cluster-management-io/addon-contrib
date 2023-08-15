import asyncio
import logging
import random

from asyncua import Server, ua
from asyncua.common.methods import uamethod


# @uamethod
# def func(parent, value):
#     return value * 2

async def main():
    _logger = logging.getLogger(__name__)
    # setup our server
    server = Server()
    await server.init()
    server.set_endpoint("opc.tcp://0.0.0.0:4840/freeopcua/server/")

    # set up our own namespace, not really necessary but should as spec
    uri = "http://examples.freeopcua.github.io"
    idx = await server.register_namespace(uri)

    # populating our address space
    # server.nodes, contains links to very common nodes like objects and root
    myobj = await server.nodes.objects.add_object(idx, "MyObject")
    
    # Set Random to be writable by clients
    varrandom = await myobj.add_variable(idx, "Random", 6.7)
    await varrandom.set_writable()

    # Set Count to be writable by clients
    varcount = await myobj.add_variable(idx, "Count", 0)
    await varcount.set_writable()

    # await server.nodes.objects.add_method(
    #     ua.NodeId("ServerTimesMethod", idx),
    #     ua.QualifiedName("ServerTimesMethod", idx),
    #     func,
    #     [ua.VariantType.Int64],
    #     [ua.VariantType.Int64],
    # )

    _logger.info("Starting server!")
    _logger.info("Provide random variable: %s", varrandom)
    _logger.info("Provide count variable: %s", varcount)

    # simulator test data
    async with server:
        while True:
            await asyncio.sleep(5)
            new_val = await varrandom.get_value() + random.random() + random.randint(-1,1)
            await varrandom.write_value(new_val)
            _logger.debug("Set value of %s to %.1f", varrandom, new_val)

            new_other_val = await varcount.get_value() + 1
            await varcount.write_value(new_other_val)
            _logger.debug("Set other value of %s to %d", varcount, new_other_val)
            


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    asyncio.run(main(), debug=False)
    
